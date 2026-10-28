package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ravenseed/internal/config"
	"ravenseed/internal/database"
	"ravenseed/internal/models"
	"ravenseed/internal/releases"
	"ravenseed/internal/search"
)

type fixture struct {
	t       *testing.T
	api     *httptest.Server
	store   *database.Store
	service *search.Service
	calls   atomic.Int32
	failure atomic.Int32
	addMode atomic.Int32
	adds    chan map[string]string
}

func newFixture(t *testing.T, auth bool) *fixture {
	t.Helper()
	f := &fixture{t: t, adds: make(chan map[string]string, 10)}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "secret-qui-key" {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/instances" {
			_ = json.NewEncoder(w).Encode([]models.QuiInstance{{ID: 7, Name: "Test client", Connected: true, IsActive: true}, {ID: 8, Name: "Offline", IsActive: true}})
			return
		}
		if r.URL.Path == "/api/instances/7/categories" {
			_, _ = io.WriteString(w, `{"TV/Anime":{"name":"TV/Anime","savePath":"/tv"},"Movies":{"name":"Movies","savePath":"/movies"}}`)
			return
		}
		if r.URL.Path == "/api/instances/8/categories" {
			w.WriteHeader(503)
			_, _ = io.WriteString(w, "secret-qui-key")
			return
		}
		if r.URL.Path == "/api/instances/7/torrents" {
			if r.Method != "POST" || r.ParseMultipartForm(1<<20) != nil {
				w.WriteHeader(400)
				return
			}
			values := map[string]string{}
			for key := range r.MultipartForm.Value {
				values[key] = r.FormValue(key)
			}
			f.adds <- values
			if f.addMode.Load() == 1 {
				w.WriteHeader(500)
				_, _ = io.WriteString(w, "secret-qui-key private-download-token")
				return
			}
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(map[string]int{"added": 1, "failed": 0})
			return
		}
		if r.URL.Path == "/api/torznab/indexers" {
			_ = json.NewEncoder(w).Encode([]models.Indexer{{ID: 1, Name: "A", Enabled: true}, {ID: 2, Name: "B", Enabled: true}, {ID: 3, Name: "Target", Enabled: true}})
			return
		}
		if r.URL.Path != "/api/torznab/search" {
			w.WriteHeader(404)
			return
		}
		f.calls.Add(1)
		var q struct {
			IDs        []int  `json:"indexer_ids"`
			IMDb       string `json:"imdb_id"`
			TMDb       int    `json:"tmdb_id"`
			Season     *int   `json:"season"`
			Episode    *int   `json:"episode"`
			Categories []int  `json:"categories"`
			Offset     int    `json:"offset"`
		}
		if json.NewDecoder(r.Body).Decode(&q) != nil || len(q.IDs) != 1 {
			w.WriteHeader(400)
			return
		}
		id := q.IDs[0]
		if id == 3 {
			switch f.failure.Load() {
			case 1:
				w.WriteHeader(502)
				_, _ = io.WriteString(w, "secret-qui-key")
				return
			case 2:
				select {
				case <-r.Context().Done():
					return
				case <-time.After(time.Second):
				}
			case 3:
				_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}, "partial": true, "total": 0})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}, "total": 0})
			return
		}
		title := "The.Matrix.1999.2160p.UHD.BluRay.REMUX.HEVC.TrueHD.7.1-FraMeSToR"
		if q.Season != nil {
			title = fmt.Sprintf("Example.Show.S%02d.1080p.WEB-DL.H.264-NTb", *q.Season)
			if q.Episode != nil {
				title = fmt.Sprintf("Example.Show.S%02dE%02d.1080p.WEB-DL.H.264-NTb", *q.Season, *q.Episode)
			}
			if len(q.Categories) != 1 || q.Categories[0] != 5000 {
				t.Error("TV category not forwarded")
			}
		}
		row := map[string]any{"indexer_id": id, "title": title, "size": int64(82123456789), "seeders": 10, "search_imdb_id": q.IMDb, "search_tmdb_id": q.TMDb, "guid": "result", "publish_date": "2026-09-20T12:00:00Z"}
		row["download_url"] = "https://tracker.example/download?id=1&token=private-download-token"
		row["download_volume_factor"] = id - 1
		row["upload_volume_factor"] = 2
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{row, row}, "total": 2})
	}))
	t.Cleanup(upstream.Close)
	store, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	f.store = store
	t.Cleanup(func() { store.DB.Close() })
	c := config.Config{QuiURL: upstream.URL, APIKey: "secret-qui-key", Concurrency: 2, MaxJobs: 4, DefaultCoverage: 2, SearchTimeout: 350 * time.Millisecond, CacheTTL: time.Hour, AuthMode: "none", APIToken: "a-test-token-with-at-least-24-characters"}
	if auth {
		c.AuthMode = "token"
	}
	f.service, err = search.New(store, c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.service.Close)
	f.api = httptest.NewServer(New(f.service, t.TempDir()))
	t.Cleanup(f.api.Close)
	return f
}
func (f *fixture) do(method, path string, body any, token string) (int, []byte) {
	f.t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			f.t.Fatal(err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, f.api.URL+path, reader)
	if err != nil {
		f.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		f.t.Fatal(err)
	}
	return res.StatusCode, b
}
func (f *fixture) start(req models.SearchRequest) models.Job {
	f.t.Helper()
	code, b := f.do("POST", "/api/v1/searches", req, "")
	if code != 202 && code != 200 {
		f.t.Fatalf("start returned %d: %s", code, b)
	}
	var j models.Job
	if err := json.Unmarshal(b, &j); err != nil {
		f.t.Fatal(err)
	}
	return j
}
func (f *fixture) wait(id string) models.Job {
	f.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		j, err := f.store.Job(id)
		if err != nil {
			f.t.Fatal(err)
		}
		if j.Status != "running" {
			return j
		}
		select {
		case <-ctx.Done():
			f.t.Fatal("job did not finish")
		case <-ticker.C:
		}
	}
}
func req() models.SearchRequest {
	r := models.SearchRequest{Media: models.Media{Type: "movie", IMDbID: "tt0133093"}, IndexerIDs: []int{3, 1, 2, 2}, TargetIndexerID: 3}
	r.Filters.MinimumCoverage = 2
	return r
}

func TestSearchPersistenceCacheAndSSE(t *testing.T) {
	f := newFixture(t, false)
	j := f.start(req())
	done := f.wait(j.ID)
	if done.Status != "completed" || done.ResultCount != 2 || done.CandidateCount != 1 {
		t.Fatalf("unexpected job: %+v", done)
	}
	code, b := f.do("GET", "/api/v1/searches/"+j.ID+"/families?minimumCoverage=2&confidence=EXACT", nil, "")
	var p Page[models.Family]
	if code != 200 || json.Unmarshal(b, &p) != nil || p.Total != 1 || p.Items[0].Coverage != 2 {
		t.Fatalf("families: %d %s", code, b)
	}
	code, b = f.do("GET", "/api/v1/searches/"+j.ID+"/results", nil, "")
	var rp Page[models.Result]
	if code != 200 || json.Unmarshal(b, &rp) != nil || rp.Total != 2 || len(rp.Items[0].Raw) == 0 || len(rp.Items[0].ParserOutput) == 0 {
		t.Fatalf("raw persistence: %s", b)
	}
	code, b = f.do("GET", "/api/v1/searches/"+j.ID+"/events", nil, "")
	if code != 200 || !bytes.Contains(b, []byte("event: indexer.completed")) || !bytes.Contains(b, []byte("event: search.completed")) {
		t.Fatalf("SSE events: %d %s", code, b)
	}
	if cached := f.start(req()); !cached.CacheHit || cached.ID != j.ID {
		t.Fatal("cache miss")
	}
	if f.calls.Load() != 3 {
		t.Fatalf("cache repeated indexer searches: %d", f.calls.Load())
	}
	fresh := req()
	fresh.Refresh = true
	f.wait(f.start(fresh).ID)
	if f.calls.Load() != 6 {
		t.Fatal("refresh did not query qui")
	}
	code, b = f.do("GET", "/api/v1/searches?limit=1&offset=1", nil, "")
	var history Page[models.Job]
	if code != 200 || json.Unmarshal(b, &history) != nil || history.Total != 2 || len(history.Items) != 1 {
		t.Fatalf("pagination: %s", b)
	}
}
func TestFailedPartialAndTimedOutTargets(t *testing.T) {
	for _, mode := range []int32{1, 2, 3} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			f := newFixture(t, false)
			f.failure.Store(mode)
			j := f.wait(f.start(req()).ID)
			if len(j.IndexerErrors) != 1 || j.CandidateCount != 0 || j.ResultCount != 2 {
				t.Fatalf("unexpected partial job: %+v", j)
			}
			if strings.Contains(j.IndexerErrors[0].Error, "secret") {
				t.Fatal("upstream body leaked")
			}
			fs, err := f.store.Families(j.ID)
			if err != nil || len(fs) != 1 || fs[0].TargetStatus.Evaluated {
				t.Fatal("failed target evaluated")
			}
			if f.start(req()).CacheHit {
				t.Fatal("partial search cached")
			}
		})
	}
}
func TestTVSeasonAndTMDb(t *testing.T) {
	f := newFixture(t, false)
	r := req()
	season := 1
	r.Media = models.Media{Type: "tv", TMDbID: 1399, Season: &season}
	j := f.wait(f.start(r).ID)
	if j.CandidateCount != 1 {
		t.Fatalf("TV query failed: %+v", j)
	}
}
func TestAuthValidationAndSecrets(t *testing.T) {
	f := newFixture(t, true)
	code, _ := f.do("GET", "/api/v1/settings", nil, "")
	if code != 401 {
		t.Fatalf("auth bypass: %d", code)
	}
	token := f.service.Config.APIToken
	code, b := f.do("GET", "/api/v1/settings", nil, token)
	if code != 200 || bytes.Contains(b, []byte("secret-qui-key")) || bytes.Contains(b, []byte(`"apiKey"`)) {
		t.Fatalf("secret exposure: %s", b)
	}
	bad := req()
	bad.Media.Season = new(int)
	code, _ = f.do("POST", "/api/v1/searches", bad, token)
	if code != 400 {
		t.Fatal("movie season accepted")
	}
	for _, path := range []string{"/api/v1/searches/missing", "/api/v1/searches/missing/families", "/api/v1/searches/missing/events"} {
		code, _ = f.do("GET", path, nil, token)
		if code != 404 {
			t.Fatalf("missing resource: %s %d", path, code)
		}
	}
	code, _ = f.do("GET", "/api/v1/searches?limit=-1", nil, token)
	if code != 400 {
		t.Fatal("invalid pagination accepted")
	}
}
func TestSettingsSavedAndKeyNotReusedAtNewURL(t *testing.T) {
	f := newFixture(t, false)
	f.service.Config.QuiURL = ""
	f.service.Config.APIKey = ""
	v := f.service.Settings()
	v.QuiURL = "http://different.invalid"
	_, err := f.service.Connection(v)
	if err == nil {
		t.Fatal("sent saved API key to new host")
	}
}
func TestRestartRecovery(t *testing.T) {
	store, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	j := models.Job{ID: "interrupted", Status: "running", StartedAt: time.Now()}
	if err = store.SaveJob(j, "key"); err != nil {
		t.Fatal(err)
	}
	if err = store.Recover(); err != nil {
		t.Fatal(err)
	}
	j, err = store.Job(j.ID)
	if err != nil || j.Status != "failed" || j.CompletedAt == nil {
		t.Fatal("interrupted search not recovered")
	}
}
func TestOpenAPICoversRoutes(t *testing.T) {
	spec := OpenAPI()
	b, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v1/settings/test", "/api/v1/searches/{id}/families/{familyId}", "/api/v1/searches/{id}/events"} {
		if !bytes.Contains(b, []byte(path)) {
			t.Fatalf("missing API route %s", path)
		}
	}
}

func TestFirstRunSetupPersistsAndRedactsKey(t *testing.T) {
	f := newFixture(t, false)
	store, err := database.Open(filepath.Join(t.TempDir(), "setup.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	c := f.service.Config
	c.QuiURL, c.APIKey = "", ""
	s, err := search.New(store, c)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	settings := models.Settings{QuiURL: f.service.Config.QuiURL, APIKey: "secret-qui-key", DefaultMinCoverage: 2, DefaultTargetIndexer: 3}
	if err = s.SaveSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	public := s.Settings()
	if public.APIKey != "" || !public.HasAPIKey {
		t.Fatal("saved key was not redacted")
	}
	stored, err := store.Settings()
	if err != nil || stored.APIKey != "secret-qui-key" || stored.DefaultTargetIndexer != 3 {
		t.Fatal("settings did not persist")
	}
	public.DefaultMinCoverage = 3
	if err = s.SaveSettings(context.Background(), public); err != nil {
		t.Fatal(err)
	}
	stored, err = store.Settings()
	if err != nil || stored.APIKey != "secret-qui-key" || stored.DefaultMinCoverage != 3 {
		t.Fatal("blank key did not preserve saved key")
	}
}

func TestExplicitNoTargetAndDefaultTarget(t *testing.T) {
	f := newFixture(t, false)
	settings := f.service.Settings()
	settings.DefaultTargetIndexer = 3
	if err := f.service.SaveSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	request := req()
	request.TargetIndexerID = 0
	j := f.wait(f.start(request).ID)
	if j.Request.TargetIndexerID != 0 || j.CandidateCount != 0 {
		t.Fatal("explicit no-target selection ignored")
	}
	body := map[string]any{"media": map[string]any{"type": "movie", "imdbId": "tt0133093"}, "indexerIds": []int{1, 2, 3}}
	code, b := f.do("POST", "/api/v1/searches", body, "")
	if code != 202 {
		t.Fatalf("start: %d %s", code, b)
	}
	if err := json.Unmarshal(b, &j); err != nil {
		t.Fatal(err)
	}
	if j.Request.TargetIndexerID != 3 {
		t.Fatal("omitted target did not use default")
	}
	f.wait(j.ID)
}

func TestMatchingRulesPersistenceAndCacheIsolation(t *testing.T) {
	f := newFixture(t, false)
	original := f.wait(f.start(req()).ID)
	rules := releases.DefaultRules()
	rules.SizeTolerancePercent = 0.5
	rules.GroupAliases = map[string]string{"OLD": "NEW"}
	code, b := f.do("PUT", "/api/v1/settings/matching", rules, "")
	if code != 200 || bytes.Contains(b, []byte("secret-qui-key")) {
		t.Fatalf("rule save: %d %s", code, b)
	}
	next := f.start(req())
	if next.CacheHit || next.ID == original.ID {
		t.Fatal("old analysis reused after rules changed")
	}
	f.wait(next.ID)
	stored, err := f.store.Settings()
	if err != nil || stored.Matching.SizeTolerancePercent != 0.5 || stored.APIKey != "" {
		t.Fatalf("rules or environment secrets persisted incorrectly: %v", err)
	}
	old, err := f.store.Job(original.ID)
	if err != nil || old.MatchingRules.SizeTolerancePercent != 0.1 {
		t.Fatal("historical rules changed")
	}
	if !f.start(req()).CacheHit {
		t.Fatal("unchanged rules prevent cache reuse")
	}
	rules.GroupAliases = map[string]string{"A": "B", "B": "A"}
	code, _ = f.do("PUT", "/api/v1/settings/matching", rules, "")
	if code != 400 {
		t.Fatal("invalid aliases accepted")
	}
	if f.service.Settings().Matching.GroupAliases["OLD"] != "NEW" {
		t.Fatal("failed validation changed saved rules")
	}
}

func TestEpisodeSearchForwardingAndValidation(t *testing.T) {
	f := newFixture(t, false)
	r := req()
	season, episode := 2, 3
	r.Media = models.Media{Type: "tv", TMDbID: 1399, Season: &season, Episode: &episode}
	j := f.wait(f.start(r).ID)
	if j.FamilyCount != 1 || j.CandidateCount != 1 {
		t.Fatalf("episode search not forwarded or filtered: %+v", j)
	}
	fs, err := f.store.Families(j.ID)
	if err != nil || len(fs) != 1 || fs[0].Canonical.Episode != 3 {
		t.Fatal("episode not normalized")
	}
	r.Media.Season = nil
	code, _ := f.do("POST", "/api/v1/searches", r, "")
	if code != 400 {
		t.Fatal("episode without season accepted")
	}
}

func TestMatchingRulesCanBeSavedBeforeSetup(t *testing.T) {
	store, err := database.Open(filepath.Join(t.TempDir(), "rules.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	c := config.Config{Concurrency: 1, MaxJobs: 1, DefaultCoverage: 2, SearchTimeout: time.Second}
	s, err := search.New(store, c)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	rules := releases.DefaultRules()
	rules.IgnoredTags = []string{"CUSTOMTAG"}
	if err = s.SaveMatching(rules); err != nil {
		t.Fatal(err)
	}
	if s.Settings().Matching.IgnoredTags[0] != "CUSTOMTAG" {
		t.Fatal("offline rules not saved")
	}
}
