package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ravenseed/internal/config"
	"ravenseed/internal/database"
	"ravenseed/internal/models"
)

// newGoneFixture serves a qui instance that holds none of the monitored
// torrents, so every check finds its torrent gone.
func newGoneFixture(t *testing.T, candidates int) (*Service, *database.Store) {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/instances":
			_ = json.NewEncoder(w).Encode([]models.QuiInstance{{ID: 7, Connected: true, IsActive: true}})
		case "/api/instances/7/torrents":
			_ = json.NewEncoder(w).Encode(map[string]any{"torrents": []any{}, "total": 0})
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(upstream.Close)
	store, err := database.Open(filepath.Join(t.TempDir(), "queue.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.DB.Close() })
	s, err := New(store, config.Config{QuiURL: upstream.URL, APIKey: "key", Concurrency: 1, MaxJobs: 1, SearchTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	if err := store.CreateMonitor(models.Monitor{ID: "m", Enabled: true, SourceIndexerID: 1, TargetIndexerID: 2, SourceTrackerHost: "a.example", QuiURL: upstream.URL, IntervalHours: 24, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < candidates; i++ {
		c := models.MonitorCandidate{ID: fmt.Sprintf("c%d", i), MonitorID: "m", InstanceID: 7, Hash: fmt.Sprintf("%040x", i), Name: "Movie.2020.1080p.WEB-DL.H.264-GRP", Status: "queued", NextCheckAt: time.Now().Add(-time.Minute)}
		if err := store.InsertMonitorCandidate(c); err != nil {
			t.Fatal(err)
		}
	}
	return s, store
}

func TestDueChecksDrainTheQueue(t *testing.T) {
	s, store := newGoneFixture(t, 5)
	s.runDueChecks()
	items, err := store.MonitorCandidates("m")
	if err != nil || len(items) != 5 {
		t.Fatalf("candidates: %v %v", items, err)
	}
	for _, v := range items {
		if v.LastCheckedAt == nil || v.Status != "gone" {
			t.Fatalf("one tick left work undone: %+v", v)
		}
	}
}

func TestGoneTwiceIsDropped(t *testing.T) {
	s, store := newGoneFixture(t, 1)
	check := func() {
		t.Helper()
		v, err := store.MonitorCandidate("c0")
		if err != nil {
			t.Fatal(err)
		}
		if ok, err := store.ClaimMonitorCandidate(v.ID); err != nil || !ok {
			t.Fatalf("claim: %v %v", ok, err)
		}
		s.checkMonitorCandidate(context.Background(), v)
	}
	check()
	if v, err := store.MonitorCandidate("c0"); err != nil || v.Status != "gone" {
		t.Fatalf("first miss: %+v %v", v, err)
	}
	check()
	if _, err := store.MonitorCandidate("c0"); err == nil {
		t.Fatal("torrent gone twice is still rechecked")
	}
}

func TestCheckKeepsChangesMadeMeanwhile(t *testing.T) {
	s, store := newGoneFixture(t, 1)
	v, _ := store.MonitorCandidate("c0")
	if ok, err := store.ClaimMonitorCandidate(v.ID); err != nil || !ok {
		t.Fatal("claim failed")
	}
	// An upload queued and a policy recheck requested while the check runs.
	if err := store.SetCandidateUploadJob(v.ID, "upload_1"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RequeuePolicyCandidates(2, time.Now()); err != nil {
		t.Fatal(err)
	}
	s.checkMonitorCandidate(context.Background(), v)
	after, err := store.MonitorCandidate(v.ID)
	if err != nil || after.UploadJobID != "upload_1" || after.LastCheckedAt == nil || after.NextCheckAt.After(time.Now()) {
		t.Fatalf("check overwrote concurrent changes: %+v %v", after, err)
	}
}

func TestCheckFailuresDoNotLeaveCandidatesChecking(t *testing.T) {
	s, store := newGoneFixture(t, 1)
	v, _ := store.MonitorCandidate("c0")
	if ok, _ := store.ClaimMonitorCandidate(v.ID); !ok {
		t.Fatal("claim failed")
	}
	// The monitor cannot be read; candidate rows still can.
	if _, err := store.DB.Exec("ALTER TABLE monitors RENAME TO monitors_unreadable"); err != nil {
		t.Fatal(err)
	}
	s.checkMonitorCandidate(context.Background(), v)
	after, err := store.MonitorCandidate(v.ID)
	if err != nil || after.Status != "error" || !after.NextCheckAt.After(time.Now()) {
		t.Fatalf("candidate stuck after a store error: %+v %v", after, err)
	}
}

func TestShutdownLeavesCheckForRestart(t *testing.T) {
	s, store := newGoneFixture(t, 1)
	v, _ := store.MonitorCandidate("c0")
	if ok, _ := store.ClaimMonitorCandidate(v.ID); !ok {
		t.Fatal("claim failed")
	}
	s.Close()
	s.checkMonitorCandidate(s.ctx, v)
	after, _ := store.MonitorCandidate(v.ID)
	if after.Status != "checking" || after.Message != "" {
		t.Fatalf("shutdown stored as a check result: %+v", after)
	}
	if err := store.RecoverMonitorCandidates(); err != nil {
		t.Fatal(err)
	}
	if after, _ = store.MonitorCandidate(v.ID); after.Status != "queued" {
		t.Fatalf("restart did not queue the check: %+v", after)
	}
}

func TestCachedMissIsNoAnswerDuringOutage(t *testing.T) {
	f := newArrFixture(t)
	now := time.Now()
	miss := models.TorrentIdentity{Hash: f.candidate.Hash, Source: models.IdentityNone, FactsVersion: identityFactsVersion, ResolvedAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := f.store.SaveTorrentIdentity(miss); err != nil {
		t.Fatal(err)
	}
	f.radarrDown.Store(true)
	if v := f.check(t); v.Status != "error" || v.IdentityError == "" {
		t.Fatalf("cached miss turned an outage into a title-only result: %+v", v)
	}
}

func TestRateLimitRetriesStop(t *testing.T) {
	f := newUploadFixture(t)
	f.mu.Lock()
	f.uniqueHashes, f.progress, f.state, f.files = true, 1, "stalledUP", true
	f.rateLimitRemaining = 1
	f.mu.Unlock()
	id := f.queue(t, "limited")
	j, err := f.s.Store.Upload(id)
	if err != nil {
		t.Fatal(err)
	}
	j.RateLimitRetries = maxRateLimitRetries
	if err := f.s.Store.SaveUpload(j); err != nil {
		t.Fatal(err)
	}
	f.s.ProcessUpload(context.Background(), id)
	if j, _ = f.s.Store.Upload(id); j.Status != "needs_review" || !strings.Contains(j.Message, "rate limiting") {
		t.Fatalf("rate-limited job retried forever: %+v", j)
	}
}
