package qui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ravenseed/internal/models"
)

func TestSearchAcceptsStringEchoedTMDbID(t *testing.T) {
	for _, echoed := range []any{"603", 603, nil, "unknown"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			row := map[string]any{"indexer_id": 22, "title": "The.Matrix.1999.1080p.BluRay.x264-GRP", "guid": "fixture", "size": 10000, "search_tmdb_id": echoed}
			_ = json.NewEncoder(w).Encode(map[string]any{"total": 1, "results": []any{row}})
		}))
		c, _ := New(server.URL, "key", time.Second)
		media := models.Media{Type: "movie", TMDbID: 603, Title: "The Matrix"}
		rows, err := c.Search(context.Background(), models.SearchRequest{Media: media}, models.Indexer{ID: 22, SearchOverride: &models.IndexerSearchOverride{Movie: "tmdb"}})
		server.Close()
		if err != nil || len(rows) != 1 {
			t.Fatalf("%v: %d rows, %v", echoed, len(rows), err)
		}
		if want := echoed == "603" || echoed == 603; rows[0].IdentityVerified != want {
			t.Errorf("%v: verified %v", echoed, rows[0].IdentityVerified)
		}
	}
}

func TestSearchRejectsMalformedIMDbIDs(t *testing.T) {
	for imdb, keep := range map[string]bool{"tt0133093": true, "0133093": true, "133093": true, "tt-133093": false, "tt0": false} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			row := map[string]any{"indexer_id": 22, "title": "The.Matrix.1999.1080p.BluRay.x264-GRP", "guid": "fixture", "size": 10000, "imdb_id": imdb}
			_ = json.NewEncoder(w).Encode(map[string]any{"total": 1, "results": []any{row}})
		}))
		c, _ := New(server.URL, "key", time.Second)
		media := models.Media{Type: "movie", IMDbID: "tt0133093", Title: "The Matrix"}
		rows, err := c.Search(context.Background(), models.SearchRequest{Media: media}, models.Indexer{ID: 22, SearchOverride: &models.IndexerSearchOverride{Movie: "imdb"}})
		server.Close()
		if err != nil || (len(rows) == 1) != keep {
			t.Errorf("%s: %d rows, %v", imdb, len(rows), err)
		}
	}
}

func TestErrorsDoNotEchoStatusOrHideCancellation(t *testing.T) {
	status := http.StatusTeapot
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
	defer server.Close()
	c, _ := New(server.URL, "key", time.Second)
	if _, err := c.Indexers(context.Background()); err == nil || strings.Contains(err.Error(), "418") {
		t.Fatalf("status echoed: %v", err)
	}
	status = http.StatusUnauthorized
	if _, err := c.Indexers(context.Background()); err == nil || !strings.Contains(err.Error(), "rejected the API key") {
		t.Fatalf("auth failure: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Indexers(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation hidden: %v", err)
	}
}

func TestTorrentsPaginationStops(t *testing.T) {
	for _, tc := range []struct {
		name  string
		pages [][]Torrent
		more  bool
		want  int
	}{
		{"short page", [][]Torrent{make([]Torrent, 1000), make([]Torrent, 3)}, false, 1003},
		{"empty page", [][]Torrent{make([]Torrent, 1000), {}}, true, 1000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				page := tc.pages[min(calls, len(tc.pages)-1)]
				calls++
				_ = json.NewEncoder(w).Encode(map[string]any{"torrents": page, "hasMore": tc.more})
			}))
			defer server.Close()
			c, _ := New(server.URL, "key", time.Second)
			got, err := c.Torrents(context.Background(), 1)
			if err != nil || len(got) != tc.want || calls != len(tc.pages) {
				t.Fatalf("%d torrents in %d calls: %v", len(got), calls, err)
			}
		})
	}
}

func TestTorrentByHashNeedsExactHash(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"torrents": []Torrent{{Hash: "OTHER"}, {Hash: "ABCDEF"}}})
	}))
	defer server.Close()
	c, _ := New(server.URL, "key", time.Second)
	if got, err := c.TorrentByHash(context.Background(), 1, "abcdef"); err != nil || got.Hash != "ABCDEF" {
		t.Fatalf("hash lookup: %+v %v", got, err)
	}
	if _, err := c.TorrentByHash(context.Background(), 1, "missing"); !errors.Is(err, ErrTorrentNotFound) {
		t.Fatalf("missing hash: %v", err)
	}
}
