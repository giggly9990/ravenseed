package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"ravenseed/internal/models"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func metadataFixture(t *testing.T) (*fixture, *atomic.Int32, *atomic.Bool) {
	f := newFixture(t, false)
	calls, down := &atomic.Int32{}, &atomic.Bool{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer metadata-secret" {
			w.WriteHeader(401)
			return
		}
		if down.Load() {
			w.WriteHeader(503)
			_, _ = w.Write([]byte("metadata-secret"))
			return
		}
		calls.Add(1)
		var value any
		switch r.URL.Path {
		case "/configuration":
			value = map[string]any{"images": map[string]string{"secure_base_url": "https://image.tmdb.org/t/p/"}}
		case "/find/tt0133093":
			value = map[string]any{"movie_results": []any{map[string]int{"id": 603}}}
		case "/find/tt0944947":
			value = map[string]any{"tv_results": []any{map[string]int{"id": 1399}}}
		case "/movie/603":
			value = map[string]any{"id": 603, "title": "The Matrix", "original_title": "The Matrix", "release_date": "1999-03-31", "poster_path": "/poster.jpg", "overview": "Movie overview", "external_ids": map[string]any{"imdb_id": "tt0133093"}}
		case "/movie/604":
			value = map[string]any{"id": 604, "title": "No external IDs", "release_date": "2000-01-01", "external_ids": map[string]any{"imdb_id": nil}}
		case "/tv/1399":
			value = map[string]any{"id": 1399, "name": "Game of Thrones", "original_name": "Game of Thrones", "first_air_date": "2011-04-17", "poster_path": "/series.jpg", "external_ids": map[string]any{"imdb_id": "tt0944947", "tvdb_id": 121361}, "seasons": []any{map[string]any{"season_number": 1, "name": "Season 1", "episode_count": 10}}}
		case "/tv/1399/season/1":
			value = map[string]any{"season_number": 1, "name": "Season 1", "poster_path": "/season.jpg", "episodes": []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}}
		default:
			w.WriteHeader(404)
			return
		}
		_ = json.NewEncoder(w).Encode(value)
	}))
	t.Cleanup(upstream.Close)
	f.service.Config.TMDbBaseURL = upstream.URL
	code, b := f.do("PUT", "/api/v1/settings/metadata", models.MetadataSettings{Token: "metadata-secret", Language: "en-US"}, "")
	if code != 200 || strings.Contains(string(b), "metadata-secret") {
		t.Fatalf("settings: %d %s", code, b)
	}
	return f, calls, down
}

func resolve(t *testing.T, f *fixture, input models.ResolveRequest) models.MediaSnapshot {
	t.Helper()
	code, b := f.do("POST", "/api/v1/media/resolve", input, "")
	var out models.MediaSnapshot
	if code != 200 || json.Unmarshal(b, &out) != nil {
		t.Fatalf("resolve: %d %s", code, b)
	}
	return out
}

func TestMetadataResolutionCacheAndSeason(t *testing.T) {
	f, calls, _ := metadataFixture(t)
	m := resolve(t, f, models.ResolveRequest{ID: "imdb:tt0133093"})
	if m.Type != "movie" || m.TMDbID != 603 || m.Year != 1999 || m.Title != "The Matrix" || m.Cached || m.Warning != "" {
		t.Fatalf("movie: %+v", m)
	}
	before := calls.Load()
	m = resolve(t, f, models.ResolveRequest{ID: "tmdb:603", Type: "movie"})
	if m.IMDbID != "tt0133093" || !m.Cached || calls.Load() != before {
		t.Fatal("canonical cache was not reused")
	}
	m = resolve(t, f, models.ResolveRequest{ID: "603", Type: "movie", Refresh: true})
	if m.Cached || calls.Load() == before {
		t.Fatal("manual refresh did not fetch metadata")
	}
	season := 1
	tv := resolve(t, f, models.ResolveRequest{ID: "tt0944947", Season: &season})
	if tv.Type != "tv" || tv.TVDbID != 121361 || tv.SeasonDetails == nil || tv.SeasonDetails.EpisodeCount != 10 || tv.SeasonDetails.PosterPath != "/season.jpg" {
		t.Fatalf("TV: %+v", tv)
	}
	before = calls.Load()
	tv = resolve(t, f, models.ResolveRequest{ID: "1399", Type: "tv", Season: &season})
	if !tv.Cached || calls.Load() != before {
		t.Fatal("season cache was not reused")
	}
	missing := resolve(t, f, models.ResolveRequest{ID: "604", Type: "movie"})
	if missing.IMDbID != "" || missing.Title != "No external IDs" {
		t.Fatal("missing external IDs broke lookup")
	}
	for _, id := range []string{"-1", "imdb:603", "tmdb:tt0133093", "ttx", "0"} {
		if code, _ := f.do("POST", "/api/v1/media/resolve", models.ResolveRequest{ID: id}, ""); code != 400 {
			t.Fatalf("invalid ID accepted: %s", id)
		}
	}
}

func TestMetadataFailureDoesNotBlockSearchAndSnapshotsPersist(t *testing.T) {
	f, _, down := metadataFixture(t)
	r := req()
	j := f.start(r)
	done := f.wait(j.ID)
	if done.Media == nil || done.Media.Title != "The Matrix" || done.Indexers[0].Strategy.Method != "imdb" {
		t.Fatal("job did not snapshot resolved identity and strategy")
	}
	down.Store(true)
	fallback := resolve(t, f, models.ResolveRequest{ID: "tt0999999", Type: "movie"})
	if fallback.IMDbID != "tt0999999" || fallback.Warning == "" || strings.Contains(fallback.Warning, "metadata-secret") {
		t.Fatal("unsafe metadata failure")
	}
	r.Media.IMDbID = "tt0999999"
	r.Refresh = true
	job := f.start(r)
	if f.wait(job.ID).Status != "completed" {
		t.Fatal("metadata outage blocked torrent search")
	}
	_, err := f.store.DB.Exec("UPDATE media_metadata SET expires_at=?", time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	stale := resolve(t, f, models.ResolveRequest{ID: "tt0133093", Type: "movie"})
	if stale.Title != "The Matrix" || !stale.Cached || !strings.Contains(stale.Warning, "stale") {
		t.Fatal("stale metadata was not used")
	}
	original, _ := f.store.Job(j.ID)
	if original.Media.Warning != "" {
		t.Fatal("metadata refresh changed historical search")
	}
}

func TestMetadataSettingsPreserveOtherSettingsAndRejectConflictingIDs(t *testing.T) {
	f, _, _ := metadataFixture(t)
	settings := f.service.Settings()
	if settings.Metadata.Token != "" || !settings.Metadata.HasToken {
		t.Fatal("metadata token was not redacted")
	}
	if code, b := f.do("PUT", "/api/v1/settings", settings, ""); code != 200 {
		t.Fatalf("connection save: %d %s", code, b)
	}
	stored, _ := f.store.Settings()
	if stored.Metadata.Token != "metadata-secret" {
		t.Fatal("connection settings overwrote TMDb token")
	}
	r := req()
	r.Media.TMDbID = 999
	if code, _ := f.do("POST", "/api/v1/searches", r, ""); code != 400 {
		t.Fatal("conflicting IDs accepted")
	}
	r.Media.TMDbID = 603
	j := f.start(r)
	if f.wait(j.ID).Media.TMDbID != 603 {
		t.Fatal("compatible IDs rejected")
	}
	if code, _ := f.do("PUT", "/api/v1/settings/metadata", models.MetadataSettings{ClearToken: true, Language: "en-US"}, ""); code != 200 {
		t.Fatal("token removal failed")
	}
	if f.service.Settings().Metadata.HasToken {
		t.Fatal("token was not cleared")
	}
}
