package qui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"ravenseed/internal/models"
	"testing"
	"time"
)

func TestSearchOverrideRequests(t *testing.T) {
	season, episode := 4, 2
	for _, tc := range []struct {
		name, method, field string
		value               any
		media               models.Media
		caps                []string
		structured          bool
	}{
		{"movie tmdb despite caps", "tmdb", "tmdb_id", float64(603), models.Media{Type: "movie", IMDbID: "tt0133093", TMDbID: 603, Title: "The Matrix"}, []string{"movie-search-imdbid"}, false},
		{"movie imdb despite caps", "imdb", "imdb_id", "tt0133093", models.Media{Type: "movie", IMDbID: "tt0133093", TMDbID: 603}, []string{"movie-search-tmdbid"}, false},
		{"title despite caps", "title", "query", "The Matrix 1999", models.Media{Type: "movie", IMDbID: "tt0133093", TMDbID: 603, Title: "The Matrix", Year: 1999}, []string{"movie-search-imdbid"}, false},
		{"missing preferred id", "tmdb", "query", "The Matrix", models.Media{Type: "movie", IMDbID: "tt0133093", Title: "The Matrix"}, []string{"movie-search-imdbid"}, false},
		{"tv unsupported parameters", "tvdb", "tvdb_id", "42", models.Media{Type: "tv", TVDbID: 42, Season: &season, Episode: &episode}, []string{"tv-search-imdbid"}, false},
		{"tv supported parameters", "imdb", "imdb_id", "tt0133093", models.Media{Type: "tv", IMDbID: "tt0133093", Season: &season, Episode: &episode}, []string{"tv-search-season", "tv-search-ep"}, true},
		{"monitor broad title", "imdb", "query", "FBI International", models.Media{Type: "tv", Title: "FBI International"}, []string{"tv-search-imdbid"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body[tc.field] != tc.value {
					t.Errorf("wanted %s=%v, got %+v", tc.field, tc.value, body)
				}
				for _, key := range []string{"imdb_id", "tmdb_id", "tvdb_id", "query"} {
					if key != tc.field && body[key] != nil {
						t.Errorf("unexpected %s", key)
					}
				}
				if (body["season"] != nil) != tc.structured || (body["episode"] != nil) != tc.structured {
					t.Errorf("wrong TV parameters: %+v", body)
				}
				_, _ = w.Write([]byte(`{"total":0,"results":[]}`))
			}))
			defer server.Close()
			c, _ := New(server.URL, "key", time.Second)
			_, err := c.Search(context.Background(), models.SearchRequest{Media: tc.media}, models.Indexer{ID: 1, Capabilities: tc.caps, SearchOverride: &models.IndexerSearchOverride{Movie: tc.method, TV: tc.method}})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestUnavailableOverrideDoesNotUseAnotherID(t *testing.T) {
	strategy, err := Strategy(models.Media{Type: "movie", IMDbID: "tt0133093"}, models.Indexer{SearchOverride: &models.IndexerSearchOverride{Movie: "tmdb"}})
	if err == nil || strategy.Method != "unsupported" {
		t.Fatalf("unexpected fallback: %+v %v", strategy, err)
	}
}
