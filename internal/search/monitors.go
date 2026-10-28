package search

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"ravenseed/internal/matching"
	"ravenseed/internal/models"
	"ravenseed/internal/policy"
	"ravenseed/internal/qui"
	"ravenseed/internal/releases"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"ravenseed/internal/database"
)

var torrentHashPattern = regexp.MustCompile(`(?i)^(?:[a-f0-9]{40}|[a-f0-9]{64})$`)
var ErrMonitorPaused = errors.New("monitor is paused; resume rechecks first")

func trackerHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
}

func (s *Service) monitorClient() (*qui.Client, models.Settings, error) {
	s.mu.RLock()
	v := s.settings
	s.mu.RUnlock()
	c, err := qui.New(v.QuiURL, v.APIKey, s.Config.SearchTimeout)
	return c, v, err
}

func (s *Service) inventory(ctx context.Context, c *qui.Client) (map[int][]qui.Torrent, error) {
	instances, err := c.Instances(ctx)
	if err != nil {
		return nil, upstream(err)
	}
	out := map[int][]qui.Torrent{}
	for _, instance := range instances {
		if !instance.IsActive || !instance.Connected {
			continue
		}
		items, err := c.Torrents(ctx, instance.ID)
		if err != nil {
			return nil, upstream(err)
		}
		out[instance.ID] = items
	}
	s.indexCrossSeeds(out)
	return out, nil
}

func (s *Service) TrackerHosts(ctx context.Context) ([]models.TrackerHost, error) {
	c, _, err := s.monitorClient()
	if err != nil {
		return nil, err
	}
	inventory, err := s.inventory(ctx, c)
	if err != nil {
		return nil, err
	}
	counts := map[string]map[int]int{}
	for instance, items := range inventory {
		for _, item := range items {
			if host := trackerHost(item.Tracker); host != "" {
				if counts[host] == nil {
					counts[host] = map[int]int{}
				}
				counts[host][instance]++
			}
		}
	}
	out := []models.TrackerHost{}
	for host, perInstance := range counts {
		v := models.TrackerHost{Host: host, InstanceCounts: perInstance}
		for _, count := range perInstance {
			v.Count += count
		}
		out = append(out, v)
	}
	slices.SortFunc(out, func(a, b models.TrackerHost) int { return strings.Compare(a.Host, b.Host) })
	return out, nil
}

// monitorCovers reports whether a monitor scans a qui instance.
func monitorCovers(m models.Monitor, instance int) bool {
	return len(m.InstanceIDs) == 0 || slices.Contains(m.InstanceIDs, instance)
}

// monitorInstances sorts and dedupes a monitor's qui instance selection.
func monitorInstances(instances []int) ([]int, error) {
	instances = slices.Compact(slices.Sorted(slices.Values(instances)))
	if len(instances) == 0 {
		return nil, errors.New("choose at least one qui instance")
	}
	return instances, nil
}

// checkInstances rejects instances missing from the inventory, which lists
// only active, connected ones.
func checkInstances(instances []int, inventory map[int][]qui.Torrent) error {
	for _, instance := range instances {
		if _, ok := inventory[instance]; !ok {
			return fmt.Errorf("qui instance %d is not active and connected", instance)
		}
	}
	return nil
}

var errMonitorOffline = errors.New("none of this monitor's qui instances is active and connected")

// monitorOnline fails a scoped monitor's scan when none of its instances is
// active and connected; the scan would otherwise succeed finding nothing.
func monitorOnline(m models.Monitor, inventory map[int][]qui.Torrent) error {
	if len(m.InstanceIDs) == 0 || slices.ContainsFunc(m.InstanceIDs, func(id int) bool { _, ok := inventory[id]; return ok }) {
		return nil
	}
	return upstream(errMonitorOffline)
}

func (s *Service) CreateMonitor(ctx context.Context, source, target int, host string, hours int, instances []int) (models.Monitor, error) {
	host = strings.ToLower(strings.TrimSpace(host))
	if source <= 0 || target <= 0 || source == target || hours < 6 || hours > 168 || host == "" || trackerHost("https://"+host) != host || strings.ContainsAny(host, "/:@? #") {
		return models.Monitor{}, errors.New("choose different source and target indexers, a tracker host, and an interval from 6 to 168 hours")
	}
	instances, err := monitorInstances(instances)
	if err != nil {
		return models.Monitor{}, err
	}
	indexers, err := s.Indexers(ctx, false)
	if err != nil {
		return models.Monitor{}, err
	}
	sourcePos := slices.IndexFunc(indexers, func(v models.Indexer) bool { return v.ID == source && v.Enabled })
	targetPos := slices.IndexFunc(indexers, func(v models.Indexer) bool { return v.ID == target && v.Enabled })
	if sourcePos < 0 || targetPos < 0 {
		return models.Monitor{}, errors.New("source and target must be enabled qui indexers")
	}
	c, settings, err := s.monitorClient()
	if err != nil {
		return models.Monitor{}, err
	}
	inventory, err := s.inventory(ctx, c)
	if err != nil {
		return models.Monitor{}, err
	}
	if err := checkInstances(instances, inventory); err != nil {
		return models.Monitor{}, err
	}
	found := false
	for _, instance := range instances {
		for _, item := range inventory[instance] {
			if trackerHost(item.Tracker) == host {
				found = true
				break
			}
		}
	}
	if !found {
		return models.Monitor{}, errors.New("tracker host was not found on a torrent in the chosen qui instances")
	}
	v := models.Monitor{ID: "mon_" + randomID(), SourceIndexerID: source, SourceIndexerName: indexers[sourcePos].Name, SourceTrackerHost: host, TargetIndexerID: target, TargetIndexerName: indexers[targetPos].Name, IntervalHours: hours, Enabled: true, QuiURL: settings.QuiURL, CreatedAt: time.Now().UTC(), InstanceIDs: instances}
	if err := s.Store.CreateMonitor(v); err != nil {
		return models.Monitor{}, errors.New("this tracker pair already exists or could not be saved")
	}
	if err := s.saveInventory(v, inventory); err != nil {
		// Without its inventory the monitor is removed, so a retry can create it.
		if _, e := s.Store.DeleteMonitor(v.ID); e != nil {
			slog.Error("could not remove incomplete monitor", "monitor", v.ID, "error", e)
		}
		return models.Monitor{}, internal(err)
	}
	return s.Store.Monitor(v.ID)
}

// SetMonitorInstances replaces the qui instances a monitor scans and, when
// enabled is set, pauses or resumes it. No instances means every instance,
// including ones added later. An instance the monitor already covers is kept
// even while offline; an added one must be active and connected. An enabled
// monitor scans the new selection at once. Candidates on dropped instances
// are queued, so their next check marks them gone and the one after removes
// them; gone candidates on re-added instances are queued for a fresh check.
func (s *Service) SetMonitorInstances(ctx context.Context, id string, instances []int, enabled *bool) (models.Monitor, error) {
	instances = slices.Compact(slices.Sorted(slices.Values(instances)))
	if len(instances) == 0 {
		instances = nil
	}
	old, err := s.Store.Monitor(id)
	if err != nil {
		return models.Monitor{}, internal(err)
	}
	c, settings, err := s.monitorClient()
	if err != nil {
		return models.Monitor{}, err
	}
	if settings.QuiURL != old.QuiURL {
		return models.Monitor{}, errors.New("qui connection changed; recreate this monitor for the new connection")
	}
	inventory, err := s.inventory(ctx, c)
	if err != nil {
		return models.Monitor{}, err
	}
	items, err := s.Store.MonitorCandidates(id)
	if err != nil {
		return models.Monitor{}, internal(err)
	}
	// An all-instances monitor covers, for keeping, the instances it found
	// torrents on.
	kept := func(instance int) bool {
		if len(old.InstanceIDs) > 0 {
			return slices.Contains(old.InstanceIDs, instance)
		}
		return slices.ContainsFunc(items, func(v models.MonitorCandidate) bool { return v.InstanceID == instance })
	}
	for _, instance := range instances {
		if _, ok := inventory[instance]; !ok && !kept(instance) {
			return models.Monitor{}, fmt.Errorf("qui instance %d is not active and connected", instance)
		}
	}
	// Under the upload lock, so a candidate upload checks coverage before or
	// after the edit, never during it.
	s.uploadMu.Lock()
	m, err := s.Store.SetMonitorInstances(id, instances, enabled)
	s.uploadMu.Unlock()
	if err != nil {
		return models.Monitor{}, internal(err)
	}
	for _, v := range items {
		was, is := monitorCovers(old, v.InstanceID), monitorCovers(m, v.InstanceID)
		if (was && !is && v.Status != "gone") || (!was && is && v.Status == "gone") {
			if err := s.Store.QueueMonitorCandidate(v.ID, time.Now()); err != nil && !errors.Is(err, database.ErrCandidateBusy) && !errors.Is(err, sql.ErrNoRows) {
				return models.Monitor{}, internal(err)
			}
		}
	}
	if m.Enabled {
		if err := s.saveInventory(m, inventory); errors.Is(err, errMonitorOffline) {
			m.LastError = err.Error()
			if err := s.Store.SaveMonitorScan(m); err != nil {
				return models.Monitor{}, internal(err)
			}
		} else if err != nil && !errors.Is(err, ErrMonitorPaused) {
			return models.Monitor{}, internal(err)
		}
	}
	return s.Store.Monitor(id)
}

func (s *Service) saveInventory(m models.Monitor, inventory map[int][]qui.Torrent) error {
	current, err := s.Store.Monitor(m.ID)
	if err != nil {
		return err
	}
	if !current.Enabled {
		return ErrMonitorPaused
	}
	// The selection may have changed since the scan began.
	if err := monitorOnline(current, inventory); err != nil {
		return err
	}
	known, err := s.Store.MonitorCandidateHashes(m.ID)
	if err != nil {
		return err
	}
	added := []models.MonitorCandidate{}
	for instance, items := range inventory {
		if !monitorCovers(current, instance) {
			continue
		}
		for _, item := range items {
			if trackerHost(item.Tracker) != m.SourceTrackerHost || !torrentHashPattern.MatchString(item.Hash) {
				continue
			}
			hash := strings.ToLower(item.Hash)
			if key := fmt.Sprintf("%d|%s", instance, hash); known[key] {
				continue
			} else {
				known[key] = true
			}
			size := item.TotalSize
			if size <= 0 {
				size = item.Size
			}
			added = append(added, models.MonitorCandidate{ID: "cand_" + randomID(), MonitorID: m.ID, InstanceID: instance, Hash: hash, Name: item.Name, Size: size, Status: "queued", Message: "Queued for target search", NextCheckAt: time.Now().UTC()})
		}
	}
	if len(added) > 0 {
		if err := s.Store.InsertMonitorCandidates(added); err != nil {
			return err
		}
	}
	now := time.Now().UTC()
	m.LastScanAt, m.LastError = &now, ""
	return s.Store.SaveMonitorScan(m)
}

// ScanMonitor refreshes a monitor's inventory. qui failures are returned as
// UpstreamError and store failures as internal errors.
func (s *Service) ScanMonitor(ctx context.Context, id string) error {
	m, err := s.Store.Monitor(id)
	if err != nil {
		return internal(err)
	}
	if !m.Enabled {
		return ErrMonitorPaused
	}
	c, settings, err := s.monitorClient()
	if err == nil && settings.QuiURL != m.QuiURL {
		err = errors.New("qui connection changed; recreate this monitor for the new connection")
	}
	if err == nil {
		var inventory map[int][]qui.Torrent
		if inventory, err = s.inventory(ctx, c); err != nil {
			err = upstream(err)
		} else if err = s.saveInventory(m, inventory); err != nil && !errors.Is(err, ErrMonitorPaused) && !errors.Is(err, errMonitorOffline) {
			err = internal(err)
		}
	}
	// A cancelled scan (shutdown or an abandoned request) is not a monitor fault.
	if err != nil && !errors.Is(err, ErrMonitorPaused) && !errors.Is(err, context.Canceled) {
		m.LastError = err.Error()
		_ = s.Store.SaveMonitorScan(m)
	}
	return err
}

func (s *Service) scanAllMonitors(ctx context.Context) {
	monitors, err := s.Store.Monitors()
	if err != nil {
		slog.Error("could not load monitors", "error", err)
		return
	}
	active := []models.Monitor{}
	for _, m := range monitors {
		if m.Enabled {
			active = append(active, m)
		}
	}
	if len(active) == 0 {
		return
	}
	c, settings, err := s.monitorClient()
	var inventory map[int][]qui.Torrent
	if err == nil {
		inventory, err = s.inventory(ctx, c)
	}
	for _, m := range active {
		result := err
		if result == nil && settings.QuiURL != m.QuiURL {
			result = errors.New("qui connection changed; recreate this monitor for the new connection")
		}
		if result == nil {
			result = s.saveInventory(m, inventory)
		}
		if result != nil && !errors.Is(result, ErrMonitorPaused) && !errors.Is(result, context.Canceled) {
			m.LastError = result.Error()
			_ = s.Store.SaveMonitorScan(m)
			slog.Warn("monitor inventory scan failed", "monitor", m.ID, "error", result)
		}
	}
}

// monitorKind picks the media type before IDs are known. Episode markers in
// the name win; a TV category covers packs named without them.
func monitorKind(name, category string) string {
	if releases.Episodic(name) || mediaKindHint(category) == "tv" {
		return "tv"
	}
	return "movie"
}

// monitorMedia parses the source torrent. With a resolved identity it parses
// under those IDs; a history-bound identity marks the source verified, and
// any identity lets verified target listings match by ID across titles.
func monitorMedia(source *models.Result, rules models.MatchingRules, kind string, identity *models.TorrentIdentity) models.Media {
	ids := models.Media{Type: kind}
	if identity != nil {
		ids = identity.Media
		source.IdentityVerified = identity.Verified()
	}
	parse := models.Media{Type: ids.Type, IMDbID: ids.IMDbID, TMDbID: ids.TMDbID, TVDbID: ids.TVDbID}
	releases.ParseWithRules(source, parse, rules)
	if identity == nil && source.Parsed.MediaType == "tv" && parse.Type != "tv" {
		releases.ParseWithRules(source, models.Media{Type: "tv"}, rules)
	}
	if identity != nil && identity.Verified() && source.Parsed.Resolution == "" && identity.Resolution != "" {
		// SD names usually omit the resolution; download history records it.
		source.Parsed.Resolution = identity.Resolution
		source.NormalizationEvidence = append(source.NormalizationEvidence, "arr_history_resolution")
	}
	p := source.Parsed
	m := models.Media{Type: p.MediaType, Title: p.Title, Year: p.Year}
	if identity != nil && p.MediaType == ids.Type {
		m.IMDbID, m.TMDbID, m.TVDbID = ids.IMDbID, ids.TMDbID, ids.TVDbID
		if key := releases.MediaKey(m); key != "" && rules.MatchAlternateTitles && source.Parsed.ContentID == "" {
			source.Parsed.ContentID = key
			source.NormalizationEvidence = append(source.NormalizationEvidence, "arr_media_identity")
		}
	}
	if m.Type == "tv" {
		if p.Season > 0 {
			season := p.Season
			m.Season = &season
		}
		if p.Episode > 0 && m.Season != nil {
			episode := p.Episode
			m.Episode = &episode
		}
	}
	return m
}

type monitorAttempt struct {
	media     models.Media
	indexer   models.Indexer
	ifNoScope bool
}

// monitorAttempts searches by ID when IDs are known, then by title: tracker
// listings can lack IDs. Title searches keep IDs, which exclude listings with
// contradicting IDs, only when the identity is bound to the torrent hash: a
// wrong name-based ID must not hide the real listing. A movie title search
// without the year runs only when nothing in scope was found; an off-by-one
// tracker year then turns a same-size listing into a review instead of a
// candidate.
func monitorAttempts(media models.Media, idx models.Indexer, verified bool) []monitorAttempt {
	if media.Type == "tv" {
		// Tracker season naming is not uniform: search the series and scope locally.
		media.Season, media.Episode, media.Year = nil, nil, 0
	}
	titleMedia := media
	if !verified {
		titleMedia.IMDbID, titleMedia.TMDbID, titleMedia.TVDbID = "", 0, 0
	}
	byTitle := idx
	byTitle.SearchOverride = &models.IndexerSearchOverride{Movie: "title", TV: "title"}
	out := []monitorAttempt{{media: titleMedia, indexer: idx}}
	if strategy, err := qui.Strategy(media, idx); err == nil && strategy.Method != "title" {
		out = []monitorAttempt{{media: media, indexer: idx}}
		if _, err := qui.Strategy(titleMedia, byTitle); err == nil {
			out = append(out, monitorAttempt{media: titleMedia, indexer: byTitle})
		}
	}
	if media.Type == "movie" && media.Year > 0 {
		noYear := titleMedia
		noYear.Year = 0
		if _, err := qui.Strategy(noYear, byTitle); err == nil {
			out = append(out, monitorAttempt{media: noYear, indexer: byTitle, ifNoScope: true})
		}
	}
	return out
}

func describeStrategy(v models.SearchStrategy) string {
	if v.Method == "title" {
		return v.Query
	}
	return strings.ToUpper(v.Method) + " " + v.Query
}

// keepConcurrentChanges carries over what other requests changed while a
// check ran: an upload job queued or released by retention, an upgrade
// dismissal (which lapses when the offer changed) and an earlier due time from
// a recheck queued meanwhile.
func keepConcurrentChanges(due time.Time) func(stored models.MonitorCandidate, v *models.MonitorCandidate) {
	return func(stored models.MonitorCandidate, v *models.MonitorCandidate) {
		v.UploadJobID = stored.UploadJobID
		v.UpgradeDismissed = stored.UpgradeDismissed
		if v.UpgradeOffer != nil && v.UpgradeDismissed != "" && v.UpgradeDismissed != upgradeKey(v.UpgradeOffer) {
			v.UpgradeDismissed = ""
		}
		if !stored.NextCheckAt.Equal(due) && stored.NextCheckAt.Before(v.NextCheckAt) {
			v.NextCheckAt = stored.NextCheckAt
		}
	}
}

func (s *Service) checkMonitorCandidate(ctx context.Context, v models.MonitorCandidate) {
	due, wasGone := v.NextCheckAt, v.Status == "gone"
	removed := false
	defer func() {
		// At shutdown the row stays "checking" and is queued again at startup.
		if removed || s.ctx.Err() != nil {
			return
		}
		if err := s.Store.FinishMonitorCheck(v, keepConcurrentChanges(due)); err != nil {
			slog.Error("could not save monitor check", "candidate", v.ID, "error", err)
		}
	}()
	m, err := s.Store.Monitor(v.MonitorID)
	if errors.Is(err, sql.ErrNoRows) {
		removed = true // the monitor and its candidates were deleted
		return
	}
	if err != nil {
		v.Status, v.Message = "error", "Could not load the monitor; retrying later"
		v.NextCheckAt = time.Now().UTC().Add(time.Hour)
		return
	}
	if !m.Enabled {
		v.Status, v.Message = "queued", "Monitor paused; check will resume later"
		return
	}
	// A torrent gone at two checks in a row is dropped; a later inventory scan
	// adds it again if it returns.
	gone := func(message string) {
		if wasGone && v.UploadJobID == "" {
			if ok, err := s.Store.DeleteCheckedCandidate(v.ID); err == nil && ok {
				removed = true
				return
			}
		}
		v.Status, v.Message = "gone", message
	}
	now := time.Now().UTC()
	v.LastCheckedAt = &now
	v.NextCheckAt = now.Add(time.Duration(m.IntervalHours) * time.Hour)
	v.MatchCount = 0
	v.TargetQuery = ""
	v.TargetResultCount, v.ScopedResultCount = 0, 0
	v.Policy = nil
	v.Upgrade, v.UpgradeOffer = "", nil
	if !monitorCovers(m, v.InstanceID) {
		gone("The monitor no longer scans this torrent's qui instance")
		return
	}
	c, settings, err := s.monitorClient()
	if err == nil && settings.QuiURL != m.QuiURL {
		err = errors.New("qui connection changed; recreate this monitor")
	}
	if err != nil {
		v.Status, v.Message = "error", err.Error()
		return
	}
	item, err := c.TorrentByHash(ctx, v.InstanceID, v.Hash)
	if err != nil {
		if errors.Is(err, qui.ErrTorrentNotFound) {
			gone("Torrent is no longer available in qui")
		} else {
			v.Status, v.Message = "error", err.Error()
			v.NextCheckAt = now.Add(time.Hour)
		}
		return
	}
	if trackerHost(item.Tracker) != m.SourceTrackerHost {
		gone("Torrent no longer belongs to the selected source tracker")
		return
	}
	v.Name = item.Name
	v.Size = item.TotalSize
	if v.Size <= 0 {
		v.Size = item.Size
	}
	v.Category = item.Category
	source := models.Result{ID: v.Hash, IndexerID: m.SourceIndexerID, Title: v.Name, Size: v.Size}
	rules, _ := releases.NormalizeRules(settings.Matching)
	kind := monitorKind(v.Name, v.Category)
	v.Media = monitorMedia(&source, rules, kind, nil)
	v.Identity, v.IdentityError = nil, ""
	files := func() ([]qui.TorrentFile, error) { return c.Files(ctx, v.InstanceID, v.Hash) }
	crossSeeds := func() []string { return s.crossSeeds(ctx, c, v.Hash) }
	identity, idErr := s.torrentIdentity(ctx, v.Hash, crossSeeds, files, v.Name, v.Media.Type, source.Parsed)
	if idErr != nil {
		if ctx.Err() != nil {
			v.Status, v.Message = "error", ctx.Err().Error()
			v.NextCheckAt = now.Add(time.Hour)
			return
		}
		v.IdentityError = idErr.Error()
		slog.Warn("could not resolve torrent identity", "candidate", v.ID, "error", idErr)
	}
	v.Identity = identity
	if identity != nil && identity.Found() {
		withIDs := models.Result{ID: source.ID, IndexerID: source.IndexerID, Title: source.Title, Size: source.Size}
		idRules := rules
		idRules.EpisodeMappings = append(slices.Clone(rules.EpisodeMappings), identity.Episodes...)
		if media := monitorMedia(&withIDs, idRules, identity.Media.Type, identity); media.Type == identity.Media.Type {
			source, v.Media, rules = withIDs, media, idRules
		} else {
			v.IdentityError = fmt.Sprintf("%s identifies a %s, but the release name parses as %s; IDs ignored", identity.Instance, identity.Media.Type, media.Type)
		}
	}
	v.Parsed = source.Parsed
	// A group the target refuses needs no target search.
	if configured, err := s.Store.IndexerPolicy(m.TargetIndexerID); err == nil && configured.Enabled && policy.Banned(configured, source) {
		evaluation := policy.Evaluate(configured, source, nil, true)
		v.Policy = &evaluation
		v.Status, v.Message = "not_eligible", evaluation.Reason
		return
	}
	if v.Media.Title == "" || v.Size <= 0 || (v.Media.Type == "tv" && source.Parsed.Structure == "unknown") {
		v.Status, v.Message = "uncertain", "Release name or size is insufficient for a reliable comparison"
		return
	}
	indexers, err := s.Indexers(ctx, false)
	if err != nil {
		v.Status, v.Message = "error", err.Error()
		return
	}
	pos := slices.IndexFunc(indexers, func(i models.Indexer) bool { return i.ID == m.TargetIndexerID && i.Enabled })
	if pos < 0 {
		v.Status, v.Message = "error", "Target indexer is unavailable or disabled"
		return
	}
	possible, rounded := false, false
	inScope := []models.Result{}
	present := false
	seen := map[string]bool{}
	queries := []string{}
	for _, attempt := range monitorAttempts(v.Media, indexers[pos], source.IdentityVerified) {
		if present || (attempt.ifNoScope && len(inScope) > 0) {
			break
		}
		if strategy, strategyErr := qui.Strategy(attempt.media, attempt.indexer); strategyErr == nil {
			queries = append(queries, describeStrategy(strategy))
			v.TargetQuery = strings.Join(queries, " · ")
		}
		req := models.SearchRequest{Media: attempt.media, IndexerIDs: []int{m.TargetIndexerID}, TargetIndexerID: m.TargetIndexerID, Refresh: true}
		results, err := c.Search(ctx, req, attempt.indexer)
		if err != nil {
			v.TargetResultCount += len(results)
			v.Status, v.Message = "error", err.Error()
			v.NextCheckAt = now.Add(time.Hour)
			return
		}
		for i := range results {
			// Searches return the same torrent with search-specific links, so
			// the result ID differs; one tracker never lists two copies.
			key := releaseKey(results[i].Size, results[i].Title)
			if seen[key] {
				continue
			}
			seen[key] = true
			v.TargetResultCount++
			releases.ParseWithRules(&results[i], v.Media, rules)
			if !releases.InScopeOrPack(results[i].Parsed, v.Media) {
				continue
			}
			inScope = append(inScope, results[i])
			confidence, _ := matching.CompareWithRules(source, results[i], rules)
			if matching.ObservedEquivalent(source, results[i]) {
				v.MatchCount++
				present = true
			}
			if results[i].Size == source.Size && confidence != "REJECTED" {
				possible = true
			}
			if matching.RoundedEquivalent(source, results[i]) {
				rounded = true
			}
		}
	}
	v.ScopedResultCount = len(inScope)
	if present {
		v.Status, v.Message = "present", "Matching release and reported byte size found on target"
	} else if rounded {
		v.Status, v.Message = "uncertain", "Target lists this release with a rounded size; compare the files before uploading"
	} else if possible {
		v.Status, v.Message = "uncertain", "A similar same-size result exists on target; review it manually"
	} else if v.IdentityError != "" && (v.Identity == nil || !v.Identity.Found()) {
		// Without the IDs an outage withheld, a title miss is not trusted.
		v.Status, v.Message = "error", "Sonarr or Radarr could not be reached; retrying before marking a candidate"
		v.NextCheckAt = now.Add(time.Hour)
		return
	} else {
		v.Status, v.Message = "candidate", "No matching release and size found on target; review before uploading"
	}
	configured, err := s.Store.IndexerPolicy(m.TargetIndexerID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !configured.Enabled) {
		return
	}
	if err != nil {
		v.Status, v.Message = "error", "Could not load target indexer policy"
		return
	}
	evaluation := policy.EvaluateWith(configured, source, inScope, true, s.monitorPolicyContext(ctx, configured, v))
	if evaluation.Classification == models.PolicyTrump && !source.IdentityVerified {
		evaluation.Classification = models.PolicyPossibleTrump
		evaluation.Reason = "Objective trump rule matched, but source media identity needs manual review"
		reason := "Existing qui torrent has no verified media ID"
		if v.Identity != nil && v.Identity.Found() && v.IdentityError == "" {
			reason = "Media ID was inferred from the release name by " + v.Identity.Instance + ", not from its download history"
		}
		evaluation.ReviewReasons = append(evaluation.ReviewReasons, reason)
	}
	if (possible || rounded) && !present && evaluation.Classification != models.PolicyEquivalent && evaluation.Classification != models.PolicyNotEligible {
		if rounded {
			evaluation.ReviewReasons = append(evaluation.ReviewReasons, "Target lists the same release with a size rounded for display; exact bytes are unavailable")
		} else {
			evaluation.ReviewReasons = append(evaluation.ReviewReasons, "A similar same-size target result needs manual identity review")
		}
		// An unevaluated release keeps the reason the policy could not judge it.
		if evaluation.Classification != models.PolicyUnevaluated {
			evaluation.Classification = models.PolicyPossibleTrump
			evaluation.Reason = "Target release identity is uncertain"
		}
	}
	v.Policy = &evaluation
	applyUpgrade(&v, configured, source, evaluation)
	switch evaluation.Classification {
	case models.PolicyEquivalent:
		v.Status = "present"
	case models.PolicyMissingSlot, models.PolicyCoexistence, models.PolicyTrump:
		v.Status = "candidate"
	case models.PolicyPossibleTrump, models.PolicyUnevaluated:
		v.Status = "uncertain"
	case models.PolicyNotEligible:
		v.Status = "not_eligible"
		if len(evaluation.CompetingResultIDs) > 0 {
			v.Status = "occupied"
		}
	}
	v.Message = evaluation.Reason
}

const (
	// monitorCheckWorkers bounds concurrent monitor checks; each runs up to
	// three target searches and Sonarr or Radarr lookups.
	monitorCheckWorkers = 2
	// monitorDrainBudget bounds how long one tick keeps starting due checks.
	monitorDrainBudget = 50 * time.Second
)

// monitorCheckTimeout covers one check: up to three target searches, each
// bounded by the qui client's own timeout, plus identity lookups.
func (s *Service) monitorCheckTimeout() time.Duration {
	return 3*s.Config.SearchTimeout + 4*arrTimeout
}

func (s *Service) StartMonitors() {
	s.monitorsStarted.Do(func() {
		s.jobMu.Lock()
		defer s.jobMu.Unlock()
		if s.ctx.Err() != nil {
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			checks := time.NewTicker(time.Minute)
			scans := time.NewTicker(10 * time.Minute)
			defer checks.Stop()
			defer scans.Stop()
			for {
				select {
				case <-s.ctx.Done():
					return
				case <-scans.C:
					ctx, cancel := context.WithTimeout(s.ctx, s.Config.SearchTimeout)
					s.scanAllMonitors(ctx)
					cancel()
					if err := s.Store.PruneTorrentIdentities(time.Now()); err != nil {
						slog.Warn("could not prune torrent identities", "error", err)
					}
				case <-checks.C:
					s.runDueChecks()
				}
			}
		}()
	})
}

// runDueChecks claims due candidates and checks them on a few workers until
// none is due or the tick's budget is spent, so large monitors keep pace.
func (s *Service) runDueChecks() {
	stop := time.Now().Add(monitorDrainBudget)
	slots := make(chan struct{}, monitorCheckWorkers)
	var wg sync.WaitGroup
	defer wg.Wait()
	for misses := 0; misses < 3 && time.Now().Before(stop); {
		select {
		case slots <- struct{}{}:
		case <-s.ctx.Done():
			return
		}
		v, err := s.Store.DueMonitorCandidate(time.Now())
		if err != nil {
			<-slots
			if !errors.Is(err, sql.ErrNoRows) && s.ctx.Err() == nil {
				slog.Error("could not read monitor queue", "error", err)
			}
			return
		}
		claimed, err := s.Store.ClaimMonitorCandidate(v.ID)
		if err != nil || !claimed {
			<-slots
			if err != nil {
				slog.Error("could not claim monitor check", "candidate", v.ID, "error", err)
				return
			}
			misses++
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			ctx, cancel := context.WithTimeout(s.ctx, s.monitorCheckTimeout())
			defer cancel()
			s.checkMonitorCandidate(ctx, v)
		}()
	}
}

// ErrCheckRunning reports a manual check requested during a running check.
var ErrCheckRunning = errors.New("candidate is already being checked")

func (s *Service) QueueMonitorCheck(id string) error {
	v, err := s.Store.MonitorCandidate(id)
	if err != nil {
		return internal(err)
	}
	m, err := s.Store.Monitor(v.MonitorID)
	if err != nil {
		return internal(err)
	}
	if !m.Enabled {
		return ErrMonitorPaused
	}
	if v.Status == "checking" {
		return ErrCheckRunning
	}
	// A manual check re-resolves name-based IDs; history-bound IDs stay cached.
	if cached, err := s.Store.TorrentIdentity(v.Hash, time.Now()); err == nil && !cached.Verified() {
		_ = s.Store.DeleteTorrentIdentity(v.Hash)
	}
	err = s.Store.QueueMonitorCandidate(id, time.Now())
	if errors.Is(err, database.ErrCandidateBusy) {
		return ErrCheckRunning
	}
	return internal(err)
}

// monitorPolicyContext supplies the runtime bitrate tiers need, from the
// torrent's Sonarr or Radarr identity and, failing that, cached TMDb metadata.
func (s *Service) monitorPolicyContext(ctx context.Context, configured models.IndexerPolicy, v models.MonitorCandidate) policy.Context {
	var identity *models.TorrentIdentity
	if v.Identity != nil && v.Identity.Found() && v.IdentityError == "" {
		identity = v.Identity
	}
	out := PolicyContext(identity, nil)
	if identity != nil && identity.Verified() {
		out.SourceVideoMbps = identity.VideoMbps
	}
	if len(configured.EncodeBitrateTiers) == 0 || (out.RuntimeMinutes > 0 && (v.Media.Type == "movie" || len(out.SeasonEpisodes) > 0)) {
		return out
	}
	id := ""
	switch {
	case v.Media.TMDbID > 0:
		id = fmt.Sprintf("tmdb:%d", v.Media.TMDbID)
	case v.Media.IMDbID != "":
		id = "imdb:" + v.Media.IMDbID
	default:
		return out
	}
	snapshot, err := s.Resolve(ctx, models.ResolveRequest{ID: id, Type: v.Media.Type})
	if err != nil {
		return out
	}
	measured := out.SourceVideoMbps
	out = PolicyContext(identity, &snapshot)
	out.SourceVideoMbps = measured
	return out
}
