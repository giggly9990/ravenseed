package search

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"regexp"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"ravenseed/internal/arr"
	"ravenseed/internal/clustering"
	"ravenseed/internal/config"
	"ravenseed/internal/database"
	"ravenseed/internal/models"
	"ravenseed/internal/policy"
	"ravenseed/internal/qui"
	"ravenseed/internal/ranking"
	"ravenseed/internal/releases"
)

var ErrBusy = errors.New("maximum active searches reached")
var imdbPattern = regexp.MustCompile(`^tt[0-9]{7,10}$`)

type Service struct {
	Store           *database.Store
	Config          config.Config
	mu              sync.RWMutex
	jobMu           sync.Mutex
	uploadMu        sync.Mutex
	arrMu           sync.Mutex
	arrGeneration   atomic.Uint64
	arrClientMu     sync.Mutex
	arrClients      map[string]*arr.Client
	siblingMu       sync.RWMutex
	siblings        map[string][]string
	siblingsAt      time.Time
	uploadStarted   sync.Once
	monitorsStarted sync.Once
	uploadSlots     chan struct{}
	uploadWake      chan struct{}
	uploadDue       time.Time
	settings        models.Settings
	indexers        []models.Indexer
	synced          time.Time
	semaphore       chan struct{}
	jobs            chan struct{}
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
}

func New(store *database.Store, c config.Config) (*Service, error) {
	v, err := store.Settings()
	if err != nil {
		return nil, err
	}
	rules, err := releases.NormalizeRules(v.Matching)
	if err != nil {
		return nil, err
	}
	v.Matching = &rules
	if v.Metadata == nil {
		v.Metadata = &models.MetadataSettings{Language: "en-US"}
	}
	if v.Metadata.Language == "" {
		v.Metadata.Language = "en-US"
	}
	if c.TMDbToken != "" {
		v.Metadata.Token = c.TMDbToken
	}
	v.Metadata.EnvironmentManaged = c.TMDbToken != ""
	if v.DefaultMinCoverage == 0 {
		v.DefaultMinCoverage = c.DefaultCoverage
	}
	if c.DefaultTarget != 0 {
		v.DefaultTargetIndexer = c.DefaultTarget
	}
	if c.QuiURL != "" {
		v.QuiURL = c.QuiURL
	}
	if c.APIKey != "" {
		v.APIKey = c.APIKey
	}
	v.EnvironmentManaged = c.QuiURL != "" || c.APIKey != ""
	ctx, cancel := context.WithCancel(context.Background())
	if c.UploadConcurrency == 0 {
		c.UploadConcurrency = 1
	}
	if c.UploadSubmissionInterval == 0 {
		c.UploadSubmissionInterval = time.Minute
	}
	s := &Service{Store: store, Config: c, settings: v, semaphore: make(chan struct{}, c.Concurrency), jobs: make(chan struct{}, c.MaxJobs), ctx: ctx, cancel: cancel}
	s.uploadSlots = make(chan struct{}, c.UploadConcurrency)
	s.uploadWake = make(chan struct{}, 1)
	if err = store.Recover(); err != nil {
		cancel()
		return nil, err
	}
	if err = store.RecoverUploads(); err != nil {
		cancel()
		return nil, err
	}
	if err = store.RecoverMonitorCandidates(); err != nil {
		cancel()
		return nil, err
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
			_, _ = s.Indexers(ctx, true)
			cancel()
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	s.startRetention()
	return s, nil
}
func (s *Service) Close() { s.jobMu.Lock(); s.cancel(); s.jobMu.Unlock(); s.wg.Wait() }
func (s *Service) Settings() models.Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v := s.settings
	rules, _ := releases.NormalizeRules(v.Matching)
	v.Matching = &rules
	v.SearchOverrides = maps.Clone(v.SearchOverrides)
	v.HasAPIKey = v.APIKey != ""
	v.APIKey = ""
	metadata := *v.Metadata
	metadata.HasToken = metadata.Token != ""
	metadata.Token = ""
	v.Metadata = &metadata
	return v
}
func (s *Service) Connection(v models.Settings) (*qui.Client, error) {
	s.mu.RLock()
	current := s.settings
	s.mu.RUnlock()
	if v.QuiURL == "" {
		v.QuiURL = current.QuiURL
	}
	if v.APIKey == "" {
		if v.QuiURL != current.QuiURL {
			return nil, errors.New("enter an API key when changing the qui URL")
		}
		v.APIKey = current.APIKey
	}
	return qui.New(v.QuiURL, v.APIKey, s.Config.SearchTimeout)
}
func (s *Service) SaveSettings(ctx context.Context, v models.Settings) error {
	if v.DefaultMinCoverage < 1 || v.DefaultMinCoverage > 1000 || v.DefaultTargetIndexer < 0 {
		return errors.New("invalid default coverage or target")
	}
	s.mu.RLock()
	current := s.settings
	s.mu.RUnlock()
	if current.EnvironmentManaged && (v.QuiURL != current.QuiURL || v.APIKey != "") {
		return errors.New("qui connection is managed by environment variables")
	}
	// Without rules in the request, the rules saved when this save commits
	// are kept, not a copy read before a concurrent rules save.
	keepRules := v.Matching == nil
	if !keepRules {
		rules, err := releases.NormalizeRules(v.Matching)
		if err != nil {
			return err
		}
		v.Matching = &rules
	}
	client, err := s.Connection(v)
	if err != nil {
		return err
	}
	idx, err := client.Indexers(ctx)
	if err != nil {
		return upstream(err)
	}
	if v.DefaultTargetIndexer != 0 && !slices.ContainsFunc(idx, func(i models.Indexer) bool { return i.Enabled && i.ID == v.DefaultTargetIndexer }) {
		return errors.New("default target must be an enabled indexer")
	}
	if v.APIKey == "" {
		v.APIKey = current.APIKey
	}
	v.EnvironmentManaged = current.EnvironmentManaged
	s.mu.Lock()
	defer s.mu.Unlock()
	v.Metadata = s.settings.Metadata
	v.SearchOverrides = s.settings.SearchOverrides
	if keepRules {
		v.Matching = s.settings.Matching
	}
	if err = s.persistSettings(v); err != nil {
		return internal(err)
	}
	s.settings = v
	s.indexers = idx
	s.synced = time.Now()
	return nil
}
func (s *Service) Indexers(ctx context.Context, refresh bool) ([]models.Indexer, error) {
	s.mu.RLock()
	v := s.settings
	if !refresh && !s.synced.IsZero() && time.Since(s.synced) < time.Minute {
		out := withSearchOverrides(s.indexers, s.settings.SearchOverrides)
		s.mu.RUnlock()
		return out, nil
	}
	s.mu.RUnlock()
	c, err := qui.New(v.QuiURL, v.APIKey, s.Config.SearchTimeout)
	if err != nil {
		return nil, err
	}
	out, err := c.Indexers(ctx)
	if err != nil {
		return nil, upstream(err)
	}
	s.mu.Lock()
	if s.settings.QuiURL == v.QuiURL && s.settings.APIKey == v.APIKey {
		s.indexers = out
		s.synced = time.Now()
	}
	out = withSearchOverrides(out, s.settings.SearchOverrides)
	s.mu.Unlock()
	return out, nil
}

func Validate(req models.SearchRequest) error {
	if req.Media.Type != "movie" && req.Media.Type != "tv" {
		return errors.New("media.type must be movie or tv")
	}
	if req.Media.IMDbID == "" && req.Media.TMDbID == 0 {
		return errors.New("provide an IMDb or TMDb ID")
	}
	if req.Media.IMDbID != "" && !imdbPattern.MatchString(req.Media.IMDbID) {
		return errors.New("invalid IMDb ID")
	}
	if req.Media.TMDbID < 0 {
		return errors.New("TMDb ID must be positive")
	}
	if req.Media.Season != nil && (req.Media.Type != "tv" || *req.Media.Season < 0 || *req.Media.Season > 999) {
		return errors.New("season requires TV and a value from 0 to 999")
	}
	if req.Media.Episode != nil && (req.Media.Type != "tv" || req.Media.Season == nil || *req.Media.Episode < 1 || *req.Media.Episode > 9999) {
		return errors.New("episode requires TV, a season and a value from 1 to 9999")
	}
	if req.Filters.MinimumCoverage < 0 || req.Filters.MinimumCoverage > 1000 {
		return errors.New("minimumCoverage must be from 1 to 1000, or 0 for the default")
	}
	if len(req.IndexerIDs) > 1000 {
		return errors.New("too many indexers")
	}
	for _, id := range req.IndexerIDs {
		if id <= 0 {
			return errors.New("indexer IDs must be positive")
		}
	}
	return nil
}

func (s *Service) Start(ctx context.Context, req models.SearchRequest) (models.Job, error) {
	if err := Validate(req); err != nil {
		return models.Job{}, err
	}
	indexers, err := s.Indexers(ctx, false)
	if err != nil {
		return models.Job{}, err
	}
	s.mu.RLock()
	settings := s.settings
	s.mu.RUnlock()
	client, err := qui.New(settings.QuiURL, settings.APIKey, s.Config.SearchTimeout)
	if err != nil {
		return models.Job{}, err
	}
	if req.Filters.MinimumCoverage == 0 {
		req.Filters.MinimumCoverage = settings.DefaultMinCoverage
	}
	if len(req.IndexerIDs) == 0 {
		for _, idx := range indexers {
			if idx.Enabled {
				req.IndexerIDs = append(req.IndexerIDs, idx.ID)
			}
		}
	}
	slices.Sort(req.IndexerIDs)
	req.IndexerIDs = slices.Compact(req.IndexerIDs)
	if len(req.IndexerIDs) == 0 {
		return models.Job{}, errors.New("no enabled indexers selected")
	}
	selected := []models.Indexer{}
	for _, id := range req.IndexerIDs {
		pos := slices.IndexFunc(indexers, func(v models.Indexer) bool { return v.ID == id && v.Enabled })
		if pos < 0 {
			return models.Job{}, fmt.Errorf("indexer %d is unavailable or disabled", id)
		}
		selected = append(selected, indexers[pos])
	}
	if req.TargetIndexerID != 0 && !slices.Contains(req.IndexerIDs, req.TargetIndexerID) {
		return models.Job{}, errors.New("target must be one of the selected indexers")
	}
	id := fmt.Sprint(req.Media.TMDbID)
	if req.Media.IMDbID != "" {
		id = req.Media.IMDbID
	}
	media, err := s.Resolve(ctx, models.ResolveRequest{ID: id, Type: req.Media.Type, Season: req.Media.Season})
	if err != nil {
		return models.Job{}, err
	}
	if req.Media.IMDbID != "" && req.Media.TMDbID > 0 && (media.Title == "" || media.TMDbID != req.Media.TMDbID) {
		return models.Job{}, errors.New("TMDb must verify the supplied IMDb and TMDb IDs refer to the same media")
	}
	media.Episode = req.Media.Episode
	req.Media.Title, req.Media.OriginalTitle, req.Media.Year, req.Media.TVDbID = "", "", 0, 0
	keyReq := req
	keyReq.Refresh = false
	policySnapshotID := ""
	if configured, policyErr := s.Store.IndexerPolicy(req.TargetIndexerID); policyErr == nil && configured.Enabled {
		policySnapshotID = configured.SnapshotID
	} else if policyErr != nil && !errors.Is(policyErr, sql.ErrNoRows) {
		return models.Job{}, internal(policyErr)
	}
	b, _ := json.Marshal(struct {
		Request          models.SearchRequest
		Connection       string
		Indexers         []models.Indexer
		Version          int
		Rules            *models.MatchingRules
		Media            models.Media
		PolicySnapshotID string
	}{keyReq, settings.QuiURL + settings.APIKey, selected, releases.EngineVersion, settings.Matching, media.Media, policySnapshotID})
	h := sha256.Sum256(b)
	key := hex.EncodeToString(h[:])
	if !req.Refresh && s.Config.CacheTTL > 0 {
		if cached, e := s.Store.Cached(key, time.Now().Add(-s.Config.CacheTTL)); e == nil {
			return cached, nil
		}
	}
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	if s.ctx.Err() != nil {
		return models.Job{}, errors.New("service is shutting down")
	}
	select {
	case s.jobs <- struct{}{}:
	default:
		return models.Job{}, ErrBusy
	}
	j := models.Job{ID: "srch_" + randomID(), Status: "running", Request: req, StartedAt: time.Now().UTC(), Indexers: []models.IndexerProgress{}, IndexerErrors: []models.IndexerError{}, PolicySnapshotID: policySnapshotID}
	j.MatchingRules = settings.Matching
	j.Media = &media
	for _, idx := range selected {
		strategy, _ := qui.Strategy(media.Media, idx)
		j.Indexers = append(j.Indexers, models.IndexerProgress{IndexerID: idx.ID, Name: idx.Name, Status: "queued", Strategy: &strategy})
	}
	if err = s.Store.SaveJob(j, key); err != nil {
		<-s.jobs
		return j, internal(err)
	}
	if err = s.Store.Event("search.started", j); err != nil {
		<-s.jobs
		j.Status = "failed"
		j.Error = "Could not persist search event"
		_ = s.Store.SaveJob(j, key)
		return j, internal(err)
	}
	workerJob := j
	workerJob.Indexers = slices.Clone(j.Indexers)
	workerJob.IndexerErrors = slices.Clone(j.IndexerErrors)
	s.wg.Add(1)
	go s.run(workerJob, key, selected, client)
	return j, nil
}
func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

// run searches every indexer and analyses the results. Each indexer search
// gets SEARCH_TIMEOUT once it holds a search slot; the whole job is bounded
// by one timeout per wave of searches the shared slots allow.
func (s *Service) run(j models.Job, key string, indexers []models.Indexer, client *qui.Client) {
	defer s.wg.Done()
	defer func() { <-s.jobs }()
	slots := max(1, cap(s.semaphore))
	waves := 1 + (len(indexers)+slots-1)/slots
	ctx, cancel := context.WithTimeout(s.ctx, time.Duration(waves)*s.Config.SearchTimeout)
	defer cancel()
	rules, _ := releases.NormalizeRules(j.MatchingRules)
	var mu sync.Mutex
	var wg sync.WaitGroup
	results := []models.Result{}
	var persistErr error
	emit := func(name string) {
		if err := s.Store.SaveJob(j, key); err != nil {
			persistErr = err
			cancel()
			return
		}
		if err := s.Store.Event(name, j); err != nil {
			persistErr = err
			cancel()
		}
	}
	for position, idx := range indexers {
		wg.Add(1)
		go func(position int, idx models.Indexer) {
			defer wg.Done()
			select {
			case s.semaphore <- struct{}{}:
				defer func() { <-s.semaphore }()
			case <-ctx.Done():
				mu.Lock()
				j.Indexers[position].Status = "failed"
				j.Indexers[position].Error = "Search timeout or shutdown"
				j.IndexerErrors = append(j.IndexerErrors, models.IndexerError{IndexerID: idx.ID, Error: j.Indexers[position].Error})
				emit("indexer.failed")
				mu.Unlock()
				return
			}
			mu.Lock()
			j.Indexers[position].Status = "running"
			emit("indexer.started")
			mu.Unlock()
			request := j.Request
			if j.Media != nil {
				request.Media = j.Media.Media
			}
			searchCtx, searchCancel := context.WithTimeout(ctx, s.Config.SearchTimeout)
			rs, err := client.Search(searchCtx, request, idx)
			searchCancel()
			for i := range rs {
				releases.ParseWithRules(&rs[i], request.Media, rules)
			}
			mu.Lock()
			defer mu.Unlock()
			results = append(results, rs...)
			p := &j.Indexers[position]
			p.ResultCount = len(rs)
			j.ResultCount += len(rs)
			if err != nil {
				p.Status = "failed"
				p.Error = err.Error()
				j.IndexerErrors = append(j.IndexerErrors, models.IndexerError{IndexerID: idx.ID, Error: p.Error})
				emit("indexer.failed")
			} else {
				p.Status = "completed"
				emit("indexer.completed")
			}
			slog.Info("indexer search finished", "search_id", j.ID, "indexer_id", idx.ID, "query_method", p.Strategy.Method, "result_count", len(rs), "status", p.Status)
		}(position, idx)
	}
	wg.Wait()
	emit("analysis.started")
	eligible := []models.Result{}
	for _, r := range results {
		if !releases.InScope(r.Parsed, j.Request.Media) {
			continue
		}
		eligible = append(eligible, r)
	}
	families := clustering.BuildWithRules(eligible, j.Request, j.Indexers, rules)
	var configured models.IndexerPolicy
	policyErr := sql.ErrNoRows // no policy applies without a snapshot
	if j.PolicySnapshotID != "" {
		configured, policyErr = s.Store.PolicySnapshot(j.PolicySnapshotID)
	}
	if policyErr == nil && configured.Enabled {
		// Season packs on the target hold a searched episode's season too.
		target := []models.Result{}
		for _, result := range results {
			if result.IndexerID == j.Request.TargetIndexerID && releases.InScopeOrPack(result.Parsed, j.Request.Media) {
				target = append(target, result)
			}
		}
		complete := slices.ContainsFunc(j.Indexers, func(v models.IndexerProgress) bool {
			return v.IndexerID == j.Request.TargetIndexerID && v.Status == "completed"
		})
		for i := range families {
			for k := range families[i].Variants {
				variant := &families[i].Variants[k]
				for _, result := range families[i].Results {
					if result.IndexerID != j.Request.TargetIndexerID && slices.Contains(variant.ResultIDs, result.ID) {
						evaluation := policy.EvaluateWith(configured, result, target, complete, PolicyContext(nil, j.Media))
						// Size variants are EXACT or POSSIBLE.
						if variant.Confidence != "EXACT" && (evaluation.Classification == models.PolicyTrump || evaluation.Classification == models.PolicyMissingSlot || evaluation.Classification == models.PolicyCoexistence) {
							evaluation.Classification = models.PolicyPossibleTrump
							evaluation.Reason = "Release match confidence needs manual review"
							evaluation.ReviewReasons = append(evaluation.ReviewReasons, "This size variant has "+variant.Confidence+" matching confidence")
						}
						variant.Policy = &evaluation
						if families[i].Policy == nil {
							families[i].Policy = &evaluation
						}
						break
					}
				}
			}
		}
	}
	ranking.Sort(families, "coverage")
	j.FamilyCount = len(families)
	for _, f := range families {
		if f.TargetStatus.Candidate {
			j.CandidateCount++
		}
	}
	emit("analysis.completed")
	j.Status = "completed"
	if len(j.IndexerErrors) == len(indexers) {
		j.Status = "failed"
		j.Error = "All selected indexers failed"
	}
	if persistErr != nil {
		j.Status = "failed"
		j.Error = "Could not persist search progress"
	}
	now := time.Now().UTC()
	j.CompletedAt = &now
	if err := s.Store.SaveAnalysis(j, results, families); err != nil {
		j.Status = "failed"
		j.Error = "Could not persist search analysis"
		_ = s.Store.SaveJob(j, key)
		slog.Error("analysis persistence failed", "search_id", j.ID)
		_ = s.Store.Event("search.completed", j)
	}
	slog.Info("search finished", "search_id", j.ID, "media_id", fmt.Sprintf("%s/%d", j.Request.Media.IMDbID, j.Request.Media.TMDbID), "duration", time.Since(j.StartedAt).String(), "result_count", j.ResultCount, "family_count", j.FamilyCount, "candidate_count", j.CandidateCount)
}
