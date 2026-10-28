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

func TestPerIndexerStrategy(t *testing.T) {
	season := 1
	media := models.Media{Type: "tv", TVDbID: 121361, IMDbID: "tt0944947", TMDbID: 1399, Title: "Game of Thrones", Year: 2011, Season: &season}
	for _, tc := range []struct {
		caps   []string
		method string
	}{{[]string{"tv-search-tvdbid", "tv-search-season"}, "tvdb"}, {[]string{"tv-search-imdbid", "tv-search-season"}, "imdb"}, {[]string{"tv-search-tmdbid", "tv-search-season"}, "tmdb"}, {[]string{"tv-search-q"}, "title"}, {[]string{"tv-search-imdbid"}, "unsupported"}} {
		strategy, err := Strategy(media, models.Indexer{Capabilities: tc.caps})
		if strategy.Method != tc.method || (err != nil) != (tc.method == "unsupported") {
			t.Fatalf("strategy: %+v %v", strategy, err)
		}
	}
}

func TestTitleFallbackRequiresResultIdentityAndRejectsConflicts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input map[string]any
		_ = json.NewDecoder(r.Body).Decode(&input)
		if input["query"] != "The Matrix 1999" || input["imdb_id"] != nil || input["tmdb_id"] != nil {
			t.Errorf("incorrect fallback query: %+v", input)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"total": 3, "results": []any{
			map[string]any{"title": "Unknown", "guid": "1", "search_imdb_id": "tt0133093"},
			map[string]any{"title": "Verified", "guid": "2", "imdb_id": "tt0133093"},
			map[string]any{"title": "Conflict", "guid": "3", "imdb_id": "tt0133093", "tmdb_id": "999"},
		}})
	}))
	defer server.Close()
	c, _ := New(server.URL, "key", time.Second)
	rows, err := c.Search(context.Background(), models.SearchRequest{Media: models.Media{Type: "movie", IMDbID: "tt0133093", TMDbID: 603, Title: "The Matrix", Year: 1999}}, models.Indexer{ID: 1, Capabilities: []string{"movie-search-q"}})
	if err != nil || len(rows) != 2 || rows[0].IdentityVerified || !rows[1].IdentityVerified {
		t.Fatalf("identity checks: %+v %v", rows, err)
	}
}
