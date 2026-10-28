package metadata

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCancellationIsSurfaced(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{}`)) }))
	defer server.Close()
	c, _ := New(server.URL, "private-token", "en-US")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.Test(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation reported as %v", err)
	}
}

func TestSeasonMismatchIsRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"season_number":2,"name":"Season 2","episodes":[{},{}]}`))
	}))
	defer server.Close()
	c, _ := New(server.URL, "private-token", "en-US")
	if _, _, err := c.Season(context.Background(), 1399, 1); err == nil {
		t.Fatal("another season accepted")
	}
	season, _, err := c.Season(context.Background(), 1399, 2)
	if err != nil || season.EpisodeCount != 2 {
		t.Fatalf("season: %+v %v", season, err)
	}
}
