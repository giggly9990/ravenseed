package qui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"ravenseed/internal/models"
	"ravenseed/internal/releases"
	"strings"
	"testing"
	"time"
)

func TestRetiredTVDbRequiresTwoActualMatchingPrimaryIDs(t *testing.T) {
	for _, tc := range []struct {
		name          string
		imdb          any
		tmdb          any
		tvdb          any
		mediaType     string
		keep, warning bool
	}{
		{"Blood of Zeus retired TVDb", "10009170", "111111", "361140", "tv", true, true},
		{"numeric IDs", "tt10009170", 111111, 361140, "tv", true, true},
		{"matching TVDb", "10009170", "111111", "473467", "tv", true, false},
		{"unknown TVDb", "10009170", "111111", "0", "tv", true, false},
		{"null TVDb", "10009170", "111111", nil, "tv", true, false},
		{"missing IMDb", "", "111111", "361140", "tv", false, false},
		{"missing TMDb", "10009170", nil, "361140", "tv", false, false},
		{"conflicting IMDb", "9999999", "111111", "361140", "tv", false, false},
		{"conflicting TMDb", "10009170", "999999", "361140", "tv", false, false},
		{"movie remains strict", "10009170", "111111", "361140", "movie", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				row := map[string]any{"indexer_id": 22, "title": "Blood of Zeus S01 1080p NF WEB-DL DD+ 5.1 H.264-MZABI", "guid": "fixture", "size": 10000, "imdb_id": tc.imdb, "tmdb_id": tc.tmdb, "tvdb_id": tc.tvdb, "search_imdb_id": "tt10009170", "search_tmdb_id": 111111, "search_tvdb_id": "473467"}
				_ = json.NewEncoder(w).Encode(map[string]any{"total": 1, "results": []any{row}})
			}))
			defer server.Close()
			c, _ := New(server.URL, "key", time.Second)
			media := models.Media{Type: tc.mediaType, IMDbID: "tt10009170", TMDbID: 111111, TVDbID: 473467, Title: "Blood of Zeus"}
			rows, err := c.Search(context.Background(), models.SearchRequest{Media: media}, models.Indexer{ID: 22, SearchOverride: &models.IndexerSearchOverride{Movie: "imdb", TV: "tmdb"}})
			if err != nil {
				t.Fatal(err)
			}
			if (len(rows) == 1) != tc.keep {
				t.Fatalf("unexpected acceptance: %d", len(rows))
			}
			if !tc.keep {
				return
			}
			if !rows[0].IdentityVerified {
				t.Fatal("matching primary IDs not verified")
			}
			// Parsing must preserve the identity warning and unmodified original IDs.
			releases.Parse(&rows[0], media)
			if (len(rows[0].IdentityEvidence) > 0) != tc.warning {
				t.Fatalf("wrong identity evidence: %v", rows[0].IdentityEvidence)
			}
			if tc.warning && (!strings.Contains(rows[0].IdentityEvidence[0], "361140") || !strings.Contains(rows[0].IdentityEvidence[0], "473467") || !strings.Contains(string(rows[0].Raw), "361140")) {
				t.Fatal("ID discrepancy was erased")
			}
		})
	}
}
