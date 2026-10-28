package qui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"ravenseed/internal/models"
	"strings"
	"testing"
	"time"
)

func TestMissingTrackerFactorsRemainUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{map[string]any{"title": "Unknown", "guid": "unknown"}, map[string]any{"title": "Free", "guid": "free", "download_volume_factor": 0, "upload_volume_factor": 2}}, "total": 2})
	}))
	defer server.Close()
	c, _ := New(server.URL, "key", time.Second)
	results, err := c.Search(context.Background(), models.SearchRequest{Media: models.Media{Type: "movie", IMDbID: "tt0133093"}}, models.Indexer{ID: 1})
	if err != nil || len(results) != 2 {
		t.Fatalf("search: %v", err)
	}
	if results[0].DownloadFactor != nil || results[0].UploadFactor != nil {
		t.Fatal("missing factors became known")
	}
	if results[1].DownloadFactor == nil || *results[1].DownloadFactor != 0 {
		t.Fatal("freeleech lost")
	}
}

func TestTorrentHandoffResponseAndBasePath(t *testing.T) {
	for _, response := range []string{`{"added":1,"failed":0}`, `{"added":0,"failed":0}`, `{"added":1,"failed":1}`} {
		t.Run(response, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/qui/api/instances/7/torrents" || r.Header.Get("X-API-Key") != "key" {
					t.Error("incorrect destination or authentication")
				}
				w.WriteHeader(201)
				_, _ = w.Write([]byte(response))
			}))
			defer server.Close()
			c, _ := New(server.URL+"/qui", "key", time.Second)
			_, err := c.AddTorrent(context.Background(), models.Result{DownloadURL: "magnet:?xt=urn:btih:abc", IndexerID: 3}, models.AddTorrentRequest{InstanceID: 7})
			if (err == nil) != strings.Contains(response, `"added":1,"failed":0`) {
				t.Fatalf("unexpected result: %v", err)
			}
		})
	}
}

func TestTorrentHandoffRejectsUnusableLinks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("invalid link forwarded") }))
	defer server.Close()
	c, _ := New(server.URL, "key", time.Second)
	for _, link := range []string{"", "file:///test.torrent", "https://tracker.example/one,https://tracker.example/two", "https://tracker.example/one\nhttps://tracker.example/two", "https://user:pass@tracker.example/torrent"} {
		if _, err := c.AddTorrent(context.Background(), models.Result{DownloadURL: link, IndexerID: 1}, models.AddTorrentRequest{InstanceID: 7}); err == nil {
			t.Fatal("invalid link accepted")
		}
	}
}
