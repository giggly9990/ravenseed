package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"ravenseed/internal/config"
	"ravenseed/internal/database"
	"ravenseed/internal/search"
	"sync/atomic"
	"testing"
	"time"
)

func TestPublicHealthDoesNotContactUnavailableQui(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "private upstream failure", 503)
	}))
	defer upstream.Close()
	store, err := database.Open(filepath.Join(t.TempDir(), "health.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	s, err := search.New(store, config.Config{QuiURL: upstream.URL, APIKey: "private-key", Concurrency: 1, MaxJobs: 1, SearchTimeout: time.Second, AuthMode: "token", APIToken: "a-test-token-with-at-least-24-characters"})
	if err != nil {
		t.Fatal(err)
	}
	// Isolate HTTP-triggered requests from background polling.
	s.Close()
	calls.Store(0)
	handler := New(s, t.TempDir())
	request := func(path, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	for i := 0; i < 250; i++ {
		w := request("/api/v1/health", "")
		var body map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if w.Code != 200 || body["status"] != "ok" || body["database"] != "ok" || len(body) != 2 {
			t.Fatalf("public health: %d %s", w.Code, w.Body.String())
		}
	}
	for _, token := range []string{"", "invalid"} {
		if w := request("/api/v1/ready", token); w.Code != 401 {
			t.Fatalf("unauthenticated readiness: %d", w.Code)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("public or rejected request contacted qui")
	}
	for i := 0; i < 240; i++ {
		w := request("/api/v1/ready", s.Config.APIToken)
		if w.Code != 503 || w.Body.String() != "{\"database\":\"ok\",\"qui\":\"unavailable\",\"status\":\"degraded\"}\n" {
			t.Fatalf("unavailable readiness: %d %s", w.Code, w.Body.String())
		}
	}
	before := calls.Load()
	if before == 0 {
		t.Fatal("authorized readiness did not check qui")
	}
	if w := request("/api/v1/ready", s.Config.APIToken); w.Code != 429 {
		t.Fatalf("readiness not rate-limited: %d", w.Code)
	}
	if w := request("/api/v1/health", ""); w.Code != 200 {
		t.Fatal("API quota blocked Docker healthcheck")
	}
	if calls.Load() != before {
		t.Fatal("rate-limited or public request contacted qui")
	}
	store.DB.Close()
	if w := request("/api/v1/health", ""); w.Code != 503 {
		t.Fatal("database failure not reported")
	}
}

func TestReadinessReportsConnectedQuiAndDatabaseFailure(t *testing.T) {
	f := newFixture(t, true)
	code, body := f.do("GET", "/api/v1/ready", nil, f.service.Config.APIToken)
	var result map[string]string
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	if code != 200 || result["qui"] != "connected" {
		t.Fatalf("readiness: %d %s", code, body)
	}
	f.service.Close()
	f.store.DB.Close()
	code, _ = f.do("GET", "/api/v1/ready", nil, f.service.Config.APIToken)
	if code != 503 {
		t.Fatal("readiness ignored database failure")
	}
}
