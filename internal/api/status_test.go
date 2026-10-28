package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"ravenseed/internal/models"
	"ravenseed/internal/search"
)

func TestOpenAPIDocumentsEveryRoute(t *testing.T) {
	f := newFixture(t, false)
	router, ok := New(f.service, t.TempDir()).(chi.Routes)
	if !ok {
		t.Fatal("router does not expose its routes")
	}
	paths := OpenAPI()["paths"].(map[string]any)
	err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if !strings.HasPrefix(route, "/api/v1/") {
			return nil // login, health aliases, the spec and docs pages
		}
		operations, ok := paths[route].(map[string]any)
		if !ok || operations[strings.ToLower(method)] == nil {
			return fmt.Errorf("%s %s is not documented", method, route)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	responses := func(path, method string) map[string]any {
		return paths[path].(map[string]any)[method].(map[string]any)["responses"].(map[string]any)
	}
	if responses("/api/v1/monitor-candidates/{candidateId}/upgrade/dismiss", "post")["202"] != nil || responses("/api/v1/monitor-candidates/{candidateId}/check", "post")["202"] == nil {
		t.Fatal("202 documented for the wrong candidate actions")
	}
}

func TestServiceErrorStatuses(t *testing.T) {
	f := newFixture(t, false)
	if err := f.store.CreateMonitor(models.Monitor{ID: "mon1", SourceIndexerID: 1, SourceTrackerHost: "a.example", TargetIndexerID: 2, IntervalHours: 24, Enabled: false, CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	c := models.MonitorCandidate{ID: "c1", MonitorID: "mon1", InstanceID: 7, Hash: strings.Repeat("a", 40), Name: "Show.S01E01.1080p.WEB-DL.H.264-GRP", Status: "queued", NextCheckAt: time.Now()}
	if _, err := f.store.DB.Exec("INSERT INTO monitor_candidates VALUES (?,?,?,?,?,?,?)", c.ID, c.MonitorID, c.InstanceID, c.Hash, c.Status, "2026-01-01T00:00:00.000000000Z", `{"id":"c1","monitorId":"mon1","status":"queued"}`); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		method, path string
		body         any
		want         int
	}{
		{"POST", "/api/v1/monitors/mon1/scan", map[string]any{}, 409},
		{"POST", "/api/v1/monitors/missing/scan", map[string]any{}, 404},
		{"POST", "/api/v1/monitor-candidates/c1/check", map[string]any{}, 409},
		{"POST", "/api/v1/monitor-candidates/missing/check", map[string]any{}, 404},
		{"DELETE", "/api/v1/presets/missing", nil, 404},
		{"POST", "/api/v1/uploads/missing/cancel", map[string]any{}, 404},
		{"POST", "/api/v1/uploads/missing/retry", map[string]any{"attempt": 0}, 404},
		{"POST", "/api/v1/monitor-candidates/missing/upgrade/dismiss", map[string]bool{"dismissed": true}, 404},
		{"POST", "/api/v1/indexer-policies/9/recheck", nil, 404},
		{"GET", "/api/v1/upgrades?q=" + strings.Repeat("x", 201), nil, 400},
		// An upgrade goes to a connected instance only.
		{"POST", "/api/v1/monitor-candidates/c1/upgrade/add", map[string]any{"instanceId": 8}, 400},
	} {
		if code, body := f.do(test.method, test.path, test.body, ""); code != test.want {
			t.Errorf("%s %s: %d, want %d: %s", test.method, test.path, code, test.want, body)
		}
	}
	// A failing upstream is a gateway error, not a rejected input.
	f.service.Config.TMDbBaseURL = "http://127.0.0.1:1"
	if code, body := f.do("POST", "/api/v1/settings/metadata/test", map[string]any{"token": "x", "language": "en-US"}, ""); code != 502 {
		t.Errorf("metadata test upstream failure: %d %s", code, body)
	}
	big := map[string]string{"name": strings.Repeat("x", 70<<10)}
	if code, _ := f.do("PUT", "/api/v1/presets", big, ""); code != 413 {
		t.Errorf("oversized body: %d", code)
	}
}

func TestCandidateGroupSearchIgnoresSeparators(t *testing.T) {
	f := newFixture(t, false)
	if err := f.store.CreateMonitor(models.Monitor{ID: "mon1", SourceIndexerID: 1, SourceTrackerHost: "a.example", TargetIndexerID: 2, IntervalHours: 24, Enabled: true, CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	for i, group := range []string{"D-Z0N3", "YTS.MX", "GRP"} {
		c := models.MonitorCandidate{ID: fmt.Sprintf("c%d", i), MonitorID: "mon1", InstanceID: 7, Hash: fmt.Sprintf("%040d", i), Name: fmt.Sprintf("Movie%d", i), Status: "queued", NextCheckAt: time.Now(), Upgrade: "possible", UpgradeOffer: &models.PolicyTargetResult{Title: fmt.Sprintf("Other%d", i), Size: 1}}
		c.Parsed.Group = group
		if err := f.store.InsertMonitorCandidate(c); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"/api/v1/monitors/mon1/candidates", "/api/v1/upgrades"} {
		for q, want := range map[string]string{"D-Z0N3": "c0", "d.z0n3": "c0", "YTS.MX": "c1", "yts mx": "c1"} {
			code, body := f.do("GET", path+"?q="+url.QueryEscape(q), nil, "")
			var page struct {
				Items []models.MonitorCandidate `json:"items"`
			}
			if code != 200 || json.Unmarshal(body, &page) != nil || len(page.Items) != 1 || page.Items[0].ID != want {
				t.Errorf("%s q=%q: %d %s", path, q, code, body)
			}
		}
	}
}

func TestTimeoutKeepsItsHint(t *testing.T) {
	w := httptest.NewRecorder()
	serviceError(w, search.TimeoutHint{Hint: "Check Upload queue before retrying.", Err: context.DeadlineExceeded}, "add_failed", "")
	if w.Code != 504 || !strings.Contains(w.Body.String(), "The request timed out. Check Upload queue before retrying.") {
		t.Fatalf("timeout hint: %d %s", w.Code, w.Body)
	}
}
