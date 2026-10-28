package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"ravenseed/internal/config"
	"ravenseed/internal/database"
	"ravenseed/internal/models"
	"ravenseed/internal/policy"
	"ravenseed/internal/releases"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type arrFixture struct {
	// mu guards searches and added, written by the stub server.
	mu            sync.Mutex
	s             *Service
	store         *database.Store
	candidate     models.MonitorCandidate
	searches      []map[string]any
	historyCalls  atomic.Int32
	parseCalls    atomic.Int32
	radarrDown    atomic.Bool
	historyHit    atomic.Bool
	targetByID    atomic.Bool
	targetByTitle atomic.Bool
	listedTwice   atomic.Bool
	// crossSeeded: Radarr downloaded the sibling, the monitor checks a cross-seed.
	crossSeeded atomic.Bool
	// importedSize: Radarr's import record for this file, 0 for none.
	importedSize atomic.Int64
	importCalls  atomic.Int32
	// upgrade, when set, is listed on the target by the ID search.
	upgrade     string
	upgradeSize int64
	added       []string
	// fileSize is the torrent's single file; Radarr reports it at libraryMbps.
	fileSize    int64
	libraryMbps int64
	fileCalls   atomic.Int32
	// librarySize overrides the library file's size when it differs.
	librarySize int64
}

var siblingHash = strings.Repeat("d", 40)

func (f *arrFixture) searchLog() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.searches)
}

func (f *arrFixture) addLog() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.added)
}

const amelieSource = "Amelie.2001.1080p.BluRay.x264-GRP"
const amelieTarget = "Le Fabuleux Destin d'Amélie Poulain 2001 1080p BluRay x264-GRP"

func newArrFixture(t *testing.T) *arrFixture {
	t.Helper()
	f := &arrFixture{}
	hash := strings.Repeat("c", 40)
	radarr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f.radarrDown.Load() || r.Header.Get("X-Api-Key") != "radarr-key" {
			w.WriteHeader(503)
			return
		}
		movie := `{"id":3,"title":"Amélie","year":2001,"tmdbId":194,"imdbId":"tt0211915"}`
		switch r.URL.Path {
		case "/api/v3/history":
			f.historyCalls.Add(1)
			asked := r.URL.Query().Get("downloadId")
			if asked != strings.ToUpper(hash) && asked != strings.ToUpper(siblingHash) {
				t.Errorf("history not filtered by hash: %s", r.URL.RawQuery)
			}
			downloaded := strings.ToUpper(hash)
			if f.crossSeeded.Load() {
				downloaded = strings.ToUpper(siblingHash)
			}
			records := "[]"
			if f.historyHit.Load() && asked == downloaded {
				records = `[{"downloadId":"` + downloaded + `","movieId":3,"movie":` + movie + `}]`
			}
			_, _ = w.Write([]byte(`{"records":` + records + `}`))
		case "/api/v3/history/movie":
			f.importCalls.Add(1)
			if r.URL.Query().Get("movieId") != "3" || r.URL.Query().Get("eventType") != "3" {
				t.Errorf("imports not requested for the library movie: %s", r.URL.RawQuery)
			}
			records := "[]"
			if size := f.importedSize.Load(); size > 0 {
				records = fmt.Sprintf(`[{"eventType":"downloadFolderImported","downloadId":"%s","quality":{"quality":{"resolution":1080}},"data":{"droppedPath":"/downloads/movies/%s.mkv","size":"%d"}}]`, strings.Repeat("F", 40), amelieSource, size)
			}
			_, _ = w.Write([]byte(records))
		case "/api/v3/moviefile":
			f.fileCalls.Add(1)
			size := f.fileSize
			if f.librarySize > 0 {
				size = f.librarySize
			}
			_, _ = w.Write([]byte(fmt.Sprintf(`[{"size":%d,"mediaInfo":{"videoBitrate":%d}}]`, size, f.libraryMbps)))
		case "/api/v3/parse":
			f.parseCalls.Add(1)
			_, _ = w.Write([]byte(`{"movie":` + movie + `}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(radarr.Close)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/instances":
			_ = json.NewEncoder(w).Encode([]models.QuiInstance{{ID: 7, Connected: true, IsActive: true}})
		case "/api/instances/7/torrents":
			if r.Method == http.MethodPost {
				_ = r.ParseMultipartForm(1 << 20)
				f.mu.Lock()
				f.added = append(f.added, r.FormValue("urls"))
				f.mu.Unlock()
				_ = json.NewEncoder(w).Encode(map[string]any{"added": 1, "failed": 0})
				return
			}
			rows := []any{map[string]any{"hash": hash, "name": amelieSource, "category": "radarr.cross", "total_size": 5000, "tracker": "https://source.example/announce", "content_path": "/data/movies/" + amelieSource + ".mkv"}}
			if f.crossSeeded.Load() {
				rows = append(rows,
					map[string]any{"hash": siblingHash, "name": amelieSource, "category": "radarr", "total_size": 5000, "tracker": "https://other.example/announce", "content_path": "/data/movies/" + amelieSource + ".mkv"},
					// Same path, different size: another payload, never a sibling.
					map[string]any{"hash": strings.Repeat("e", 40), "name": amelieSource, "total_size": 5001, "tracker": "https://third.example/announce", "content_path": "/data/movies/" + amelieSource + ".mkv"})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"torrents": rows, "total": len(rows)})
		case "/api/instances/7/torrents/" + hash + "/files":
			size := f.fileSize
			if size == 0 {
				size = 5000
			}
			_ = json.NewEncoder(w).Encode([]any{map[string]any{"name": amelieSource + ".mkv", "size": size, "progress": 1}})
		case "/api/torznab/indexers":
			_ = json.NewEncoder(w).Encode([]models.Indexer{{ID: 2, Name: "Target", Enabled: true, Capabilities: []string{"movie-search-imdbid", "movie-search-q"}}})
		case "/api/torznab/search":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.mu.Lock()
			f.searches = append(f.searches, body)
			f.mu.Unlock()
			rows := []any{}
			if body["imdb_id"] == "tt0211915" && f.targetByID.Load() {
				rows = append(rows, map[string]any{"indexer_id": 2, "title": amelieTarget, "size": 5000, "imdb_id": "tt0211915"})
			}
			if f.listedTwice.Load() {
				// The same torrent from both searches, with search-specific links.
				via := "id"
				if body["query"] != nil {
					via = "title"
				}
				rows = append(rows, map[string]any{"indexer_id": 2, "title": amelieTarget, "size": 4000, "imdb_id": "tt0211915", "guid": "https://t.example/torrents/9?via=" + via, "download_url": "https://t.example/download/9?via=" + via})
			}
			if f.upgrade != "" {
				rows = append(rows, map[string]any{"indexer_id": 2, "title": f.upgrade, "size": f.upgradeSize, "imdb_id": "tt0211915", "download_url": "https://target.example/download/" + fmt.Sprint(f.upgradeSize), "info_url": "https://target.example/torrents/1"})
			}
			if body["query"] != nil && f.targetByTitle.Load() {
				rows = append(rows,
					map[string]any{"indexer_id": 2, "title": "Amelie 2001 1080p BluRay x264-GRP", "size": 5000},
					map[string]any{"indexer_id": 2, "title": "Amelie.2001.1080p.BluRay.x264-GRP", "size": 5000, "guid": "other", "imdb_id": "tt9999999"})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"results": rows, "total": len(rows)})
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(upstream.Close)
	store, err := database.Open(filepath.Join(t.TempDir(), "arr.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.DB.Close() })
	s, err := New(store, config.Config{QuiURL: upstream.URL, APIKey: "key", Concurrency: 1, MaxJobs: 1, SearchTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	if _, err := s.SaveArrSettings(models.ArrSettings{Instances: []models.ArrInstance{{Name: "Radarr", Kind: "radarr", URL: radarr.URL, APIKey: "radarr-key", Enabled: true}}}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateMonitor(models.Monitor{ID: "m", Enabled: true, SourceIndexerID: 1, TargetIndexerID: 2, SourceTrackerHost: "source.example", QuiURL: upstream.URL, IntervalHours: 24, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	f.candidate = models.MonitorCandidate{ID: "c", MonitorID: "m", InstanceID: 7, Hash: hash, Name: amelieSource, Size: 5000, Status: "queued", NextCheckAt: time.Now()}
	if err := store.InsertMonitorCandidate(f.candidate); err != nil {
		t.Fatal(err)
	}
	f.s, f.store = s, store
	return f
}

func (f *arrFixture) check(t *testing.T) models.MonitorCandidate {
	t.Helper()
	f.mu.Lock()
	f.searches = nil
	f.mu.Unlock()
	// Like the worker, check the stored row: it carries earlier user actions.
	current, err := f.store.MonitorCandidate(f.candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.s.checkMonitorCandidate(context.Background(), current)
	v, err := f.store.MonitorCandidate(f.candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestMonitorUsesHistoryIDsAndCachesThemByHash(t *testing.T) {
	f := newArrFixture(t)
	f.historyHit.Store(true)
	f.targetByID.Store(true)
	v := f.check(t)
	if v.Status != "present" || v.Identity == nil || v.Identity.Source != models.IdentityArrHistory || v.Media.IMDbID != "tt0211915" || v.Media.TMDbID != 194 {
		t.Fatalf("foreign-titled listing not found by verified ID: %+v", v)
	}
	if len(f.searchLog()) != 1 || f.searchLog()[0]["imdb_id"] != "tt0211915" || v.TargetQuery != "IMDB tt0211915" {
		t.Fatalf("expected one IMDb search: %v %q", f.searchLog(), v.TargetQuery)
	}
	if v.Category != "radarr.cross" {
		t.Fatalf("category not recorded: %q", v.Category)
	}
	f.check(t)
	if err := f.s.QueueMonitorCheck(v.ID); err != nil {
		t.Fatal(err)
	}
	f.check(t)
	if f.historyCalls.Load() != 1 || f.parseCalls.Load() != 0 {
		t.Fatalf("history identity not reused: history=%d parse=%d", f.historyCalls.Load(), f.parseCalls.Load())
	}
}

func TestMonitorFallsBackToTitleWhenIDSearchMisses(t *testing.T) {
	f := newArrFixture(t)
	f.targetByTitle.Store(true)
	v := f.check(t)
	if v.Identity == nil || v.Identity.Source != models.IdentityArrParse {
		t.Fatalf("parse identity missing: %+v", v.Identity)
	}
	if v.Status != "present" || len(f.searchLog()) != 2 || f.searchLog()[1]["query"] != "Amélie 2001" && f.searchLog()[1]["query"] != "Amelie 2001" {
		t.Fatalf("ID-less listing not found by title: %+v %v", v, f.searchLog())
	}
	if v.MatchCount != 2 {
		t.Fatalf("a name-based ID must not exclude title results: %d", v.MatchCount)
	}
	if err := f.s.QueueMonitorCheck(v.ID); err != nil {
		t.Fatal(err)
	}
	f.check(t)
	if f.parseCalls.Load() != 2 {
		t.Fatalf("manual check did not refresh a name-based identity: %d", f.parseCalls.Load())
	}
}

func TestVerifiedIdentityExcludesConflictingTitleResults(t *testing.T) {
	f := newArrFixture(t)
	f.historyHit.Store(true)
	f.targetByTitle.Store(true)
	v := f.check(t)
	if v.Status != "present" || len(f.searchLog()) != 2 || v.MatchCount != 1 {
		t.Fatalf("history-bound IDs did not exclude a conflicting listing: %+v %v", v, f.searchLog())
	}
}

func TestMonitorContinuesByTitleWhenArrIsDown(t *testing.T) {
	f := newArrFixture(t)
	f.radarrDown.Store(true)
	v := f.check(t)
	if v.IdentityError == "" || v.Media.IMDbID != "" || v.Status != "error" || v.NextCheckAt.After(time.Now().Add(2*time.Hour)) {
		t.Fatalf("outage must not yield a candidate: %+v", v)
	}
	if _, err := f.store.TorrentIdentity(v.Hash, time.Now()); err == nil {
		t.Fatal("miss cached during an outage")
	}
	f.radarrDown.Store(false)
	f.historyHit.Store(true)
	f.targetByID.Store(true)
	if v = f.check(t); v.Status != "present" || v.IdentityError != "" {
		t.Fatalf("recovered instance not used: %+v", v)
	}
}

func TestArrSettingsKeepKeysAndHideThem(t *testing.T) {
	f := newArrFixture(t)
	saved, err := f.s.ArrSettings()
	if err != nil || len(saved.Instances) != 1 || saved.Instances[0].APIKey != "" || !saved.Instances[0].HasAPIKey || saved.Instances[0].ID == "" {
		t.Fatalf("settings: %+v %v", saved, err)
	}
	f.historyHit.Store(true)
	f.check(t)
	saved.Instances[0].Name = "Movies"
	if _, err = f.s.SaveArrSettings(saved); err != nil {
		t.Fatal(err)
	}
	raw, _ := f.store.ArrSettings()
	if raw.Instances[0].APIKey != "radarr-key" {
		t.Fatal("blank key did not keep the saved key")
	}
	if _, err = f.store.TorrentIdentity(f.candidate.Hash, time.Now()); err == nil {
		t.Fatal("saving instances did not clear cached identities")
	}
	if err = f.s.TestArrInstance(context.Background(), saved.Instances[0]); err == nil {
		t.Fatal("status endpoint missing but test passed")
	}
	saved.Instances[0].URL = "http://elsewhere.example"
	if _, err = f.s.SaveArrSettings(saved); err == nil {
		t.Fatal("key reused for a different URL")
	}
}

func TestMediaKindHint(t *testing.T) {
	for category, want := range map[string]string{"radarr": "movie", "tv-sonarr": "tv", "Movies-4K": "movie", "TV Shows": "tv", "sonarr.cross": "tv", "": "", "hdtv-archive": ""} {
		if got := mediaKindHint(category); got != want {
			t.Errorf("%q: got %q want %q", category, got, want)
		}
	}
	if monitorKind("The.Daily.Show.2024.10.01.1080p.WEB.h264-GRP", "") != "tv" || monitorKind("Show.Complete.1080p.WEB-DL-GRP", "tv") != "tv" || monitorKind("The.Matrix.1999.1080p.BluRay.x264-GRP", "") != "movie" {
		t.Fatal("monitor media kind")
	}
}

func TestMonitorMediaAppliesSonarrEpisodeMap(t *testing.T) {
	identity := &models.TorrentIdentity{Source: models.IdentityArrHistory, Instance: "Sonarr", Media: models.Media{Type: "tv", IMDbID: "tt2560140", TMDbID: 1429, TVDbID: 267440}, Episodes: []models.EpisodeMapping{{IMDbID: "tt2560140", TMDbID: 1429, Absolute: 81, Season: 4, Episode: 15}}}
	rules := releases.DefaultRules()
	rules.EpisodeMappings = append(rules.EpisodeMappings, identity.Episodes...)
	source := models.Result{Title: "[SubsPlease] Attack on Titan - 81 (1080p) [ABCD1234].mkv"}
	m := monitorMedia(&source, rules, "tv", identity)
	if m.Season == nil || *m.Season != 4 || m.Episode == nil || *m.Episode != 15 || source.Parsed.Structure != "episodes:s4:e15" {
		t.Fatalf("absolute episode not mapped: %+v %+v", m, source.Parsed)
	}
	if !source.IdentityVerified || source.Parsed.ContentID != "tv:imdb:tt2560140" || m.TVDbID != 267440 {
		t.Fatalf("history identity not applied: %+v", source.Parsed)
	}
	sd := models.Result{Title: "Scrubs.S01.DVDRip.x264-GRP"}
	identity = &models.TorrentIdentity{Source: models.IdentityArrHistory, Media: models.Media{Type: "tv", TVDbID: 76156}, Resolution: "480p"}
	monitorMedia(&sd, releases.DefaultRules(), "tv", identity)
	if sd.Parsed.Resolution != "480p" {
		t.Fatalf("history resolution not applied: %+v", sd.Parsed)
	}
	sd = models.Result{Title: "Scrubs.S01.DVDRip.x264-GRP"}
	identity.Source = models.IdentityArrParse
	monitorMedia(&sd, releases.DefaultRules(), "tv", identity)
	if sd.Parsed.Resolution != "" {
		t.Fatal("name-based identity supplied a resolution")
	}
}

func TestUploadReusesOnlyTrustedIdentities(t *testing.T) {
	for source, want := range map[string]bool{models.IdentityArrHistory: true, models.IdentityArrParse: false, models.IdentityArrLookup: false} {
		v := models.MonitorCandidate{Identity: &models.TorrentIdentity{Source: source, Media: models.Media{Type: "movie", TMDbID: 194}}}
		if trustedForUpload(v) != want {
			t.Errorf("%s: want %v", source, want)
		}
		v.IdentityError = "type conflict"
		if trustedForUpload(v) {
			t.Errorf("%s: conflicting identity trusted", source)
		}
	}
}

func TestUploadSendsNameMatchIDsOnlyWhenConfirmed(t *testing.T) {
	candidate := func(source string) models.MonitorCandidate {
		return models.MonitorCandidate{Media: models.Media{Type: "movie", IMDbID: "tt0211915", TMDbID: 194}, Identity: &models.TorrentIdentity{Source: source, Media: models.Media{Type: "movie", IMDbID: "tt0211915", TMDbID: 194}}}
	}
	for name, tc := range map[string]struct {
		source string
		input  models.MonitorUploadRequest
		imdb   string
		tmdb   int
	}{
		"parse unconfirmed":           {models.IdentityArrParse, models.MonitorUploadRequest{}, "", 0},
		"parse unconfirmed with tmdb": {models.IdentityArrParse, models.MonitorUploadRequest{TMDbID: 194}, "", 194},
		"parse confirmed":             {models.IdentityArrParse, models.MonitorUploadRequest{TMDbID: 194, ConfirmMediaIDs: true}, "tt0211915", 194},
		"parse confirmed, cleared":    {models.IdentityArrParse, models.MonitorUploadRequest{ConfirmMediaIDs: true, ClearMediaIDs: true}, "", 0},
		"parse confirmed, replaced":   {models.IdentityArrParse, models.MonitorUploadRequest{TMDbID: 603, ConfirmMediaIDs: true}, "", 603},
		"history":                     {models.IdentityArrHistory, models.MonitorUploadRequest{}, "tt0211915", 194},
		"history confirmed":           {models.IdentityArrHistory, models.MonitorUploadRequest{ConfirmMediaIDs: true}, "tt0211915", 194},
		"lookup confirmed":            {models.IdentityArrLookup, models.MonitorUploadRequest{ConfirmMediaIDs: true}, "", 0},
	} {
		got := uploadMedia(candidate(tc.source), tc.input)
		if got.IMDbID != tc.imdb || got.TMDbID != tc.tmdb || got.Type != "movie" {
			t.Errorf("%s: %+v", name, got)
		}
	}
	v := candidate(models.IdentityArrParse)
	v.IdentityError = "type conflict"
	if got := uploadMedia(v, models.MonitorUploadRequest{ConfirmMediaIDs: true}); got.IMDbID != "" || got.TMDbID != 0 {
		t.Fatalf("conflicting name match sent: %+v", got)
	}
}

func TestCachedNameIdentityUpgradesToHistory(t *testing.T) {
	f := newArrFixture(t)
	if v := f.check(t); v.Identity == nil || v.Identity.Source != models.IdentityArrParse {
		t.Fatalf("expected a name-based identity first: %+v", v.Identity)
	}
	f.historyHit.Store(true)
	v := f.check(t)
	if v.Identity == nil || v.Identity.Source != models.IdentityArrHistory || f.parseCalls.Load() != 1 {
		t.Fatalf("later import was not verified from history: %+v parse=%d", v.Identity, f.parseCalls.Load())
	}
	cached, err := f.store.TorrentIdentity(v.Hash, time.Now())
	if err != nil || !cached.Verified() {
		t.Fatalf("verified identity not cached: %+v %v", cached, err)
	}
}

func TestUploadMediaHonoursClearedAndEnteredIDs(t *testing.T) {
	v := models.MonitorCandidate{Media: models.Media{Type: "movie", IMDbID: "tt0211915", TMDbID: 194}, Identity: &models.TorrentIdentity{Source: models.IdentityArrHistory, Media: models.Media{Type: "movie", TMDbID: 194}}}
	for name, tc := range map[string]struct {
		input models.MonitorUploadRequest
		imdb  string
		tmdb  int
	}{
		"prefilled":          {models.MonitorUploadRequest{TMDbID: 194}, "tt0211915", 194},
		"left unset":         {models.MonitorUploadRequest{}, "tt0211915", 194},
		"cleared":            {models.MonitorUploadRequest{ClearMediaIDs: true}, "", 0},
		"replaced":           {models.MonitorUploadRequest{TMDbID: 603}, "", 603},
		"cleared then typed": {models.MonitorUploadRequest{TMDbID: 603, ClearMediaIDs: true}, "", 603},
	} {
		got := uploadMedia(v, tc.input)
		if got.IMDbID != tc.imdb || got.TMDbID != tc.tmdb || got.Type != "movie" {
			t.Errorf("%s: %+v", name, got)
		}
	}
	v.Identity.Source = models.IdentityArrLookup
	if got := uploadMedia(v, models.MonitorUploadRequest{}); got.IMDbID != "" || got.TMDbID != 0 {
		t.Fatalf("untrusted IDs sent: %+v", got)
	}
}

func TestListingFoundByBothSearchesCountsOnce(t *testing.T) {
	f := newArrFixture(t)
	f.historyHit.Store(true)
	f.listedTwice.Store(true)
	v := f.check(t)
	if len(f.searchLog()) != 2 {
		t.Fatalf("expected an ID and a title search: %v", f.searchLog())
	}
	if v.TargetResultCount != 1 || v.ScopedResultCount != 1 {
		t.Fatalf("one listing counted twice: results=%d in scope=%d", v.TargetResultCount, v.ScopedResultCount)
	}
}

func TestCrossSeedIsVerifiedThroughTheDownloadedCopy(t *testing.T) {
	f := newArrFixture(t)
	f.historyHit.Store(true)
	f.crossSeeded.Store(true)
	f.targetByID.Store(true)
	v := f.check(t)
	if v.Identity == nil || v.Identity.Source != models.IdentityArrHistory || v.Identity.ViaHash != siblingHash {
		t.Fatalf("cross-seed not verified through its sibling: %+v", v.Identity)
	}
	if v.Status != "present" || f.parseCalls.Load() != 0 {
		t.Fatalf("verified cross-seed fell back to a name match: %+v parse=%d", v, f.parseCalls.Load())
	}
	if got := f.s.crossSeeds(context.Background(), nil, siblingHash); len(got) != 1 || got[0] != f.candidate.Hash {
		t.Fatalf("different-size payload grouped as a cross-seed: %v", got)
	}
}

func TestImportedFileVerifiesAfterTheDownloadIsGone(t *testing.T) {
	f := newArrFixture(t)
	f.importedSize.Store(5000)
	f.targetByID.Store(true)
	v := f.check(t)
	if v.Identity == nil || v.Identity.Source != models.IdentityArrHistory || v.Identity.ImportedFile != amelieSource+".mkv" || v.Identity.ViaHash != strings.Repeat("f", 40) {
		t.Fatalf("imported file did not verify the torrent: %+v", v.Identity)
	}
	if v.Identity.Resolution != "1080p" || v.Identity.ArrItemID != 3 || v.Status != "present" {
		t.Fatalf("import details: %+v status=%s", v.Identity, v.Status)
	}
	cached, err := f.store.TorrentIdentity(v.Hash, time.Now())
	if err != nil || !cached.Verified() {
		t.Fatalf("verified import not cached: %+v %v", cached, err)
	}
	f.check(t)
	if f.importCalls.Load() != 1 {
		t.Fatalf("verified identity re-read imports: %d", f.importCalls.Load())
	}
}

func TestImportedFileNeedsTheExactSize(t *testing.T) {
	f := newArrFixture(t)
	f.importedSize.Store(5001)
	v := f.check(t)
	if v.Identity == nil || v.Identity.Source != models.IdentityArrParse || v.Identity.ArrItemID != 3 {
		t.Fatalf("a same-named file of another size verified the torrent: %+v", v.Identity)
	}
	// The name-based identity keeps checking: a later matching import verifies it.
	f.importedSize.Store(5000)
	if v = f.check(t); v.Identity == nil || !v.Identity.Verified() || f.parseCalls.Load() != 1 {
		t.Fatalf("cached name match not upgraded by a later import: %+v parse=%d", v.Identity, f.parseCalls.Load())
	}
}

func TestBannedGroupSkipsTheTargetSearch(t *testing.T) {
	f := newArrFixture(t)
	f.historyHit.Store(true)
	configured := policy.Presets()[3].Policy
	configured.IndexerID = 2
	configured.BannedGroups = []string{"grp"}
	if _, err := f.store.SaveIndexerPolicy(configured); err != nil {
		t.Fatal(err)
	}
	v := f.check(t)
	if v.Status != "not_eligible" || len(f.searchLog()) != 0 || v.Policy == nil || v.Policy.Classification != models.PolicyNotEligible {
		t.Fatalf("banned group searched or offered: status=%s searches=%d %+v", v.Status, len(f.searchLog()), v.Policy)
	}
	if _, err := f.s.QueueExistingUpload(context.Background(), v.ID, models.MonitorUploadRequest{InstanceID: 7}); err == nil {
		t.Fatal("banned group's release accepted for upload")
	}
}

func TestUpgradeFromTheTargetAndAddingIt(t *testing.T) {
	f := newArrFixture(t)
	f.historyHit.Store(true)
	configured := policy.Presets()[1].Policy
	configured.IndexerID = 2
	if _, err := f.store.SaveIndexerPolicy(configured); err != nil {
		t.Fatal(err)
	}
	f.upgrade, f.upgradeSize = "Amelie.2001.REPACK.1080p.BluRay.x264-GRP", 5100
	v := f.check(t)
	if v.Upgrade != UpgradeObjective || v.UpgradeOffer == nil || v.UpgradeOffer.Title != f.upgrade || len(v.UpgradeOffer.Upgrade) != 1 || v.UpgradeOffer.Upgrade[0].Type != "correction" {
		t.Fatalf("same-group REPACK on target not offered as an upgrade: %s %+v", v.Upgrade, v.UpgradeOffer)
	}
	if _, err := f.s.SetUpgradeDismissed(v.ID, true); err != nil {
		t.Fatal(err)
	}
	if v = f.check(t); v.UpgradeDismissed == "" {
		t.Fatal("dismissal lost while the offer stayed the same")
	}
	f.upgrade, f.upgradeSize = "Amelie.2001.REPACK2.1080p.BluRay.x264-GRP", 5200
	if v = f.check(t); v.UpgradeDismissed != "" || v.UpgradeOffer.Title != f.upgrade {
		t.Fatalf("dismissal kept after the target's release changed: %+v", v)
	}
	out, err := f.s.AddUpgrade(context.Background(), v.ID, models.AddTorrentRequest{InstanceID: 7})
	if err != nil || out.Added != 1 || len(f.addLog()) != 1 || f.addLog()[0] != "https://target.example/download/5200" {
		t.Fatalf("upgrade not added from a fresh target listing: %+v %v %v", out, err, f.addLog())
	}
	f.upgrade, f.upgradeSize = "Amelie.2001.1080p.BluRay.x264-OTHER", 7000
	if v = f.check(t); v.Upgrade != UpgradePossible || v.UpgradeOffer.Title != f.upgrade {
		t.Fatalf("another group's release in the slot not a possible upgrade: %s %+v", v.Upgrade, v.UpgradeOffer)
	}
	configured.BannedGroups = []string{"OTHER"}
	if _, err := f.store.SaveIndexerPolicy(configured); err != nil {
		t.Fatal(err)
	}
	if v = f.check(t); v.Upgrade != "" {
		t.Fatalf("a banned group's release offered as an upgrade: %+v", v.UpgradeOffer)
	}
	f.upgrade = ""
	if v = f.check(t); v.Upgrade != "" || v.UpgradeOffer != nil {
		t.Fatalf("upgrade kept after the target listing left: %+v", v)
	}
}

func TestVerifiedTorrentCarriesItsMeasuredVideoBitrate(t *testing.T) {
	f := newArrFixture(t)
	f.historyHit.Store(true)
	f.fileSize, f.libraryMbps = 18556478093, 11996159
	v := f.check(t)
	if v.Identity == nil || v.Identity.VideoMbps < 11.99 || v.Identity.VideoMbps > 12 {
		t.Fatalf("measured bitrate not recorded: %+v", v.Identity)
	}
	f.check(t)
	if f.fileCalls.Load() != 1 {
		t.Fatalf("measured bitrate not cached: %d library reads", f.fileCalls.Load())
	}
	other := newArrFixture(t)
	other.historyHit.Store(true)
	// A library file of another size is not this torrent's file.
	other.fileSize, other.librarySize, other.libraryMbps = 70<<20, 60<<20, 9000000
	v = other.check(t)
	if v.Identity == nil || v.Identity.VideoMbps != 0 {
		t.Fatalf("bitrate taken from a library file that does not match: %+v", v.Identity)
	}
}
