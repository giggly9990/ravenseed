package search

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"ravenseed/internal/arr"
	"ravenseed/internal/models"
	"ravenseed/internal/policy"
	"ravenseed/internal/qui"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
)

// identityFactsVersion is raised when resolved identities gain new facts.
const identityFactsVersion = 2

const (
	identityHistoryTTL = 30 * 24 * time.Hour
	identityNameTTL    = 7 * 24 * time.Hour
	identityMissTTL    = 12 * time.Hour
	arrTimeout         = 15 * time.Second
)

func (s *Service) ArrSettings() (models.ArrSettings, error) {
	v, err := s.Store.ArrSettings()
	for i := range v.Instances {
		v.Instances[i].HasAPIKey = v.Instances[i].APIKey != ""
		v.Instances[i].APIKey = ""
	}
	return v, err
}

// SaveArrSettings keeps a saved API key when an instance's key is left blank
// and its URL is unchanged. Saving clears cached torrent identities.
func (s *Service) SaveArrSettings(v models.ArrSettings) (models.ArrSettings, error) {
	s.arrMu.Lock()
	defer s.arrMu.Unlock()
	old, err := s.Store.ArrSettings()
	if err != nil {
		return v, internal(err)
	}
	if v, err = withSavedArrKeys(v, old); err != nil {
		return v, err
	}
	if v, err = arr.Validate(v); err != nil {
		return v, err
	}
	for i := range v.Instances {
		if v.Instances[i].ID == "" {
			v.Instances[i].ID = "arr_" + randomID()[:12]
		}
	}
	// Saving and the generation bump happen under arrMu, which identity
	// caching also takes, so no identity from the old instances is cached
	// after the store cleared them.
	if err = s.Store.SaveArrSettings(v); err != nil {
		return v, internal(err)
	}
	s.arrGeneration.Add(1)
	return s.ArrSettings()
}

// arrClient reuses one client, and its connections, per instance settings.
func (s *Service) arrClient(x models.ArrInstance) (*arr.Client, error) {
	key := strings.Join([]string{x.ID, x.Kind, x.URL, x.APIKey}, "\x00")
	s.arrClientMu.Lock()
	defer s.arrClientMu.Unlock()
	if c := s.arrClients[key]; c != nil {
		return c, nil
	}
	c, err := arr.New(x.URL, x.APIKey, x.Kind, arrTimeout)
	if err != nil {
		return nil, err
	}
	if s.arrClients == nil || len(s.arrClients) >= 32 {
		s.arrClients = map[string]*arr.Client{}
	}
	s.arrClients[key] = c
	return c, nil
}

// TestArrInstance checks one instance as entered, using its saved key when the
// key field is blank.
func (s *Service) TestArrInstance(ctx context.Context, x models.ArrInstance) error {
	old, err := s.Store.ArrSettings()
	if err != nil {
		return internal(err)
	}
	x.Enabled = true
	v, err := withSavedArrKeys(models.ArrSettings{Instances: []models.ArrInstance{x}}, old)
	if err != nil {
		return err
	}
	if v, err = arr.Validate(v); err != nil {
		return err
	}
	x = v.Instances[0]
	c, err := arr.New(x.URL, x.APIKey, x.Kind, arrTimeout)
	if err != nil {
		return err
	}
	return upstream(c.Test(ctx))
}

func withSavedArrKeys(v, old models.ArrSettings) (models.ArrSettings, error) {
	v.Instances = slices.Clone(v.Instances)
	for i := range v.Instances {
		x := &v.Instances[i]
		if strings.TrimSpace(x.APIKey) != "" || x.ID == "" {
			continue
		}
		pos := slices.IndexFunc(old.Instances, func(o models.ArrInstance) bool { return o.ID == x.ID })
		if pos < 0 || old.Instances[pos].APIKey == "" {
			continue
		}
		if strings.TrimRight(strings.TrimSpace(x.URL), "/") != old.Instances[pos].URL || !strings.EqualFold(strings.TrimSpace(x.Kind), old.Instances[pos].Kind) {
			return v, fmt.Errorf("%s: enter the API key again when changing the URL or type", strings.TrimSpace(x.Name))
		}
		x.APIKey = old.Instances[pos].APIKey
	}
	return v, nil
}

// mediaKindHint reads qBittorrent categories such as "radarr" or "tv-sonarr".
func mediaKindHint(category string) string {
	c := strings.ToLower(category)
	switch {
	case strings.Contains(c, "sonarr"):
		return "tv"
	case strings.Contains(c, "radarr"):
		return "movie"
	}
	for _, token := range strings.FieldsFunc(c, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		switch token {
		case "tv", "tvshows", "shows", "series", "episodes":
			return "tv"
		case "movie", "movies", "film", "films":
			return "movie"
		}
	}
	return ""
}

// torrentIdentity resolves media IDs for a torrent through Sonarr and Radarr:
// download history for the hash first, then the release name matched against
// the library, then a unique exact title lookup. Results are cached by hash.
// A cached name-based identity still checks history so a later import is
// verified. It returns nil without error when no instance is enabled.
// crossSeeds and files are called at most once, and only when needed.
func (s *Service) torrentIdentity(ctx context.Context, hash string, crossSeeds func() []string, files func() ([]qui.TorrentFile, error), name, kind string, parsed models.ParsedRelease) (*models.TorrentIdentity, error) {
	generation := s.arrGeneration.Load()
	settings, err := s.Store.ArrSettings()
	if err != nil {
		return nil, err
	}
	preferred := arr.Radarr
	if kind == "tv" {
		preferred = arr.Sonarr
	}
	clients := []arrInstance{}
	for _, x := range settings.Instances {
		if !x.Enabled || x.APIKey == "" {
			continue
		}
		if c, err := s.arrClient(x); err == nil {
			clients = append(clients, arrInstance{x, c})
		}
	}
	if len(clients) == 0 {
		return nil, nil
	}
	if files != nil {
		files = sync.OnceValues(files)
	}
	siblings := sync.OnceValue(func() []string {
		if crossSeeds == nil {
			return nil
		}
		return crossSeeds()
	})
	slices.SortStableFunc(clients, func(a, b arrInstance) int {
		if (a.Kind == preferred) == (b.Kind == preferred) {
			return 0
		}
		if a.Kind == preferred {
			return -1
		}
		return 1
	})
	now := time.Now().UTC()
	r := identityResolver{s: s, hash: strings.ToLower(hash), now: now, generation: generation, failed: map[string]bool{}}
	// Cross-seeds share the payload, so the copy Sonarr or Radarr downloaded
	// identifies this torrent too.
	history := identityStep{models.IdentityArrHistory, true, func(c *arr.Client) (*arr.Match, error) {
		return historyMatch(ctx, c, append([]string{hash}, siblings()...))
	}}
	if cached, err := s.Store.TorrentIdentity(hash, now); err == nil && cached.FactsVersion >= identityFactsVersion {
		if cached.Verified() {
			return r.measure(ctx, clients, &cached, files), nil
		}
		if v, err := r.run(ctx, clients, preferred, history); v != nil || err != nil {
			return r.measure(ctx, clients, v, files), err
		}
		v := r.verifyByImport(ctx, clients, &cached, files)
		if !v.Found() && len(r.failures) > 0 {
			// A cached miss is no answer while an instance is unreachable.
			return v, errors.New(strings.Join(r.failures, "; "))
		}
		return r.measure(ctx, clients, v, files), nil
	}
	v, err := r.run(ctx, clients, preferred,
		history,
		identityStep{models.IdentityArrParse, false, func(c *arr.Client) (*arr.Match, error) { return c.Parse(ctx, name) }},
		identityStep{models.IdentityArrLookup, false, func(c *arr.Client) (*arr.Match, error) { return c.Lookup(ctx, parsed.Title, parsed.Year) }},
	)
	if v != nil {
		return r.measure(ctx, clients, r.verifyByImport(ctx, clients, v, files), files), err
	}
	if err != nil {
		return v, err
	}
	miss := models.TorrentIdentity{Hash: r.hash, Source: models.IdentityNone, ResolvedAt: now, ExpiresAt: now.Add(identityMissTTL)}
	if len(r.failures) > 0 {
		// An unreachable instance may know this torrent: do not cache the miss.
		return &miss, errors.New(strings.Join(r.failures, "; "))
	}
	r.save(miss)
	return &miss, nil
}

// historyMatch asks for the hash and its cross-seeds a few at a time and
// returns the match for the earliest hash in the list. Without a match, the
// first failure is returned.
func historyMatch(ctx context.Context, c *arr.Client, hashes []string) (*arr.Match, error) {
	matches := make([]*arr.Match, len(hashes))
	errs := make([]error, len(hashes))
	slots := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, h := range hashes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			matches[i], errs[i] = c.History(ctx, h)
		}()
	}
	wg.Wait()
	for _, match := range matches {
		if match != nil {
			return match, nil
		}
	}
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return nil, nil
}

type arrInstance struct {
	models.ArrInstance
	client *arr.Client
}

// instanceFor finds the instance an identity came from: by ID, or by name for
// identities cached before IDs were recorded.
func instanceFor(clients []arrInstance, v *models.TorrentIdentity) int {
	return slices.IndexFunc(clients, func(x arrInstance) bool {
		if v.InstanceID != "" {
			return x.ID == v.InstanceID
		}
		return x.Name == v.Instance
	})
}

type identityStep struct {
	source string
	all    bool
	call   func(*arr.Client) (*arr.Match, error)
}

type identityResolver struct {
	s          *Service
	hash       string
	now        time.Time
	generation uint64
	failed     map[string]bool
	failures   []string
}

// run returns the first match. Name-based steps ask only the preferred kind;
// an instance that fails is skipped for later steps.
func (r *identityResolver) run(ctx context.Context, clients []arrInstance, preferred string, steps ...identityStep) (*models.TorrentIdentity, error) {
	for _, step := range steps {
		for _, x := range clients {
			if r.failed[x.ID] || (!step.all && x.Kind != preferred) {
				continue
			}
			match, err := step.call(x.client)
			if err != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				r.failed[x.ID] = true
				r.failures = append(r.failures, x.Name+": "+err.Error())
				continue
			}
			if match == nil {
				continue
			}
			ttl := identityNameTTL
			if step.source == models.IdentityArrHistory {
				ttl = identityHistoryTTL
			}
			v := models.TorrentIdentity{Hash: r.hash, Source: step.source, Instance: x.Name, InstanceID: x.ID, Media: match.Media, Episodes: match.Episodes, Resolution: match.Resolution, Runtime: match.Runtime, SeasonEpisodes: match.SeasonEpisodes, ResolvedAt: r.now, ExpiresAt: r.now.Add(ttl)}
			if match.DownloadID != "" && !strings.EqualFold(match.DownloadID, r.hash) {
				v.ViaHash = strings.ToLower(match.DownloadID)
			}
			v.ArrItemID = match.ItemID
			r.save(v)
			return &v, nil
		}
	}
	return nil, nil
}

// save skips identities resolved from instances that were replaced meanwhile.
func (r *identityResolver) save(v models.TorrentIdentity) {
	v.FactsVersion = identityFactsVersion
	r.s.arrMu.Lock()
	defer r.s.arrMu.Unlock()
	if r.s.arrGeneration.Load() != r.generation {
		return
	}
	if err := r.s.Store.SaveTorrentIdentity(v); err != nil {
		slog.Warn("could not cache torrent identity", "hash", r.hash, "error", err)
	}
}

// verifyByImport upgrades a name-based identity when a file in the torrent has
// the name and exact size of a file the instance imported. The import record
// outlives the downloaded torrent, and a cross-seed carries the same file
// under another hash. Failures keep the name-based identity.
func (r *identityResolver) verifyByImport(ctx context.Context, clients []arrInstance, v *models.TorrentIdentity, files func() ([]qui.TorrentFile, error)) *models.TorrentIdentity {
	if v.Verified() || v.ArrItemID <= 0 || files == nil {
		return v
	}
	pos := instanceFor(clients, v)
	if pos < 0 || r.failed[clients[pos].ID] {
		return v
	}
	imports, err := clients[pos].client.Imports(ctx, v.ArrItemID)
	if err != nil || len(imports) == 0 {
		return v
	}
	contents, err := files()
	if err != nil {
		return v
	}
	held := map[string]bool{}
	for _, f := range contents {
		name := f.Name[strings.LastIndexAny(f.Name, `/\`)+1:]
		held[fmt.Sprintf("%d|%s", f.Size, strings.ToLower(name))] = true
	}
	matched := []arr.ImportedFile{}
	for _, f := range imports {
		if held[fmt.Sprintf("%d|%s", f.Size, strings.ToLower(f.Name))] {
			matched = append(matched, f)
		}
	}
	if len(matched) == 0 {
		return v
	}
	out := *v
	out.Source, out.ImportedFile = models.IdentityArrHistory, matched[0].Name
	out.ViaHash = ""
	if !strings.EqualFold(matched[0].DownloadID, r.hash) {
		out.ViaHash = matched[0].DownloadID
	}
	out.Resolution = matched[0].Resolution
	out.Episodes = nil
	for _, f := range matched {
		if e := f.Episode; e != nil && e.Absolute > 0 && (out.Media.IMDbID != "" || out.Media.TMDbID > 0) {
			out.Episodes = append(out.Episodes, models.EpisodeMapping{IMDbID: out.Media.IMDbID, TMDbID: out.Media.TMDbID, Absolute: e.Absolute, Season: e.Season, Episode: e.Episode})
		}
	}
	out.ResolvedAt, out.ExpiresAt = r.now, r.now.Add(identityHistoryTTL)
	r.save(out)
	return &out
}

// PolicyContext gathers the runtime and episode counts encode bitrate tiers
// need. Sonarr and Radarr facts come first; TMDb fills what they lack.
func PolicyContext(identity *models.TorrentIdentity, snapshot *models.MediaSnapshot) policy.Context {
	ctx := policy.Context{SeasonEpisodes: map[int]int{}}
	if identity != nil {
		ctx.RuntimeMinutes = identity.Runtime
		for season, count := range identity.SeasonEpisodes {
			ctx.SeasonEpisodes[season] = count
		}
	}
	if snapshot != nil {
		if ctx.RuntimeMinutes <= 0 {
			ctx.RuntimeMinutes = snapshot.Runtime
		}
		for _, season := range snapshot.Seasons {
			if ctx.SeasonEpisodes[season.Number] <= 0 && season.EpisodeCount > 0 {
				ctx.SeasonEpisodes[season.Number] = season.EpisodeCount
			}
		}
	}
	return ctx
}

// measure records the video bitrate MediaInfo measured on the library file
// matching this torrent, for verified identities. Every video file of the
// torrent must match a library file by exact size; packs average by size.
func (r *identityResolver) measure(ctx context.Context, clients []arrInstance, v *models.TorrentIdentity, files func() ([]qui.TorrentFile, error)) *models.TorrentIdentity {
	if v == nil || !v.Verified() || v.ArrItemID <= 0 || v.VideoMbps > 0 || files == nil {
		return v
	}
	pos := instanceFor(clients, v)
	if pos < 0 || r.failed[clients[pos].ID] {
		return v
	}
	library, err := clients[pos].client.LibraryFiles(ctx, v.ArrItemID)
	if err != nil || len(library) == 0 {
		return v
	}
	contents, err := files()
	if err != nil {
		return v
	}
	bitrate := map[int64]int64{}
	for _, f := range library {
		bitrate[f.Size] = f.VideoBitrate
	}
	var bits, bytes float64
	for _, f := range contents {
		if f.Size < 50<<20 {
			continue // samples, subtitles and extras
		}
		rate, ok := bitrate[f.Size]
		if !ok {
			return v
		}
		bits += float64(rate) * float64(f.Size)
		bytes += float64(f.Size)
	}
	if bytes == 0 {
		return v
	}
	out := *v
	out.VideoMbps = bits / bytes / 1e6
	r.save(out)
	return &out
}
