package qui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"ravenseed/internal/models"
	"sync/atomic"
	"testing"
	"time"
)

func TestPaginationAndIdentity(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var q struct {
			Offset int    `json:"offset"`
			Cache  string `json:"cache_mode"`
		}
		_ = json.NewDecoder(r.Body).Decode(&q)
		if q.Cache != "bypass" {
			t.Error("refresh not forwarded")
		}
		rows := []any{}
		switch q.Offset {
		case 0:
			rows = append(rows, map[string]any{"title": "First", "indexer_id": 1, "size": 100, "guid": "first", "imdb_id": "0133093"})
		case 1:
			rows = append(rows, map[string]any{"title": "Wrong title", "indexer_id": 1, "size": 200, "guid": "wrong", "imdb_id": "tt9999999"}, map[string]any{"title": "Unknown identity", "indexer_id": 1, "size": 300, "guid": "unknown"})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": rows, "total": 3})
	}))
	defer server.Close()
	c, err := New(server.URL, "test", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	rs, err := c.Search(context.Background(), models.SearchRequest{Media: models.Media{Type: "movie", IMDbID: "tt0133093"}, Refresh: true}, models.Indexer{ID: 1})
	if err != nil || len(rs) != 2 || calls.Load() != 2 {
		t.Fatalf("pagination failed: %d %v", len(rs), err)
	}
	if !rs[0].IdentityVerified || rs[1].IdentityVerified {
		t.Fatal("identity evidence incorrect")
	}
}
func TestUnsupportedCapability(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("unsupported ID search sent to qui") }))
	defer server.Close()
	c, _ := New(server.URL, "test", time.Second)
	_, err := c.Search(context.Background(), models.SearchRequest{Media: models.Media{Type: "tv", TMDbID: 1399}}, models.Indexer{ID: 1, Capabilities: []string{"tv-search", "tv-search-imdbid"}})
	if err == nil {
		t.Fatal("unsupported ID silently accepted")
	}
}
func TestNoCredentialRedirect(t *testing.T) {
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("credential-bearing redirect followed") }))
	defer receiver.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, receiver.URL, 302) }))
	defer upstream.Close()
	c, _ := New(upstream.URL, "secret", time.Second)
	if _, err := c.Indexers(context.Background()); err == nil {
		t.Fatal("redirect accepted")
	}
}
func TestInvalidConnectionURLs(t *testing.T) {
	for _, url := range []string{"file:///etc/passwd", "http://user:password@host", "https://host?apikey=secret", "https://host/#fragment"} {
		if _, err := New(url, "key", time.Second); err == nil {
			t.Fatalf("accepted %s", url)
		}
	}
}
