package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"ravenseed/internal/models"
	"strings"
	"testing"
	"time"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DB.Close() })
	return s
}

func addCandidate(t *testing.T, s *Store, v models.MonitorCandidate) {
	t.Helper()
	if _, err := s.Monitor("m"); errors.Is(err, sql.ErrNoRows) {
		if err := s.CreateMonitor(models.Monitor{ID: "m", Enabled: true, SourceIndexerID: 1, TargetIndexerID: 2, SourceTrackerHost: "a.example", IntervalHours: 24}); err != nil {
			t.Fatal(err)
		}
	}
	v.MonitorID, v.InstanceID = "m", 7
	if v.Hash == "" {
		v.Hash = strings.Repeat(v.ID[:1], 40)
	}
	if err := s.InsertMonitorCandidate(v); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveMonitorCandidate(v); err != nil {
		t.Fatal(err)
	}
}

func TestTargetedCandidateUpdates(t *testing.T) {
	s := openTest(t)
	offer := &models.PolicyTargetResult{Title: "Movie.2020.1080p.BluRay.x264-NEW", Size: 5000}
	addCandidate(t, s, models.MonitorCandidate{ID: "a", Status: "candidate", Message: "keep", Upgrade: "possible", UpgradeOffer: offer, NextCheckAt: time.Now()})
	addCandidate(t, s, models.MonitorCandidate{ID: "b", Status: "checking", NextCheckAt: time.Now()})

	if err := s.SetCandidateUploadJob("a", "upload_1"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCandidateUploadJob("a", "upload_2"); !errors.Is(err, ErrCandidateBusy) {
		t.Fatalf("second upload job linked: %v", err)
	}
	if err := s.SetCandidateUploadJob("missing", "upload_3"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing candidate: %v", err)
	}
	v, err := s.SetUpgradeDismissed("a", "5000|movie", offer.Title, offer.Size)
	if err != nil || v.UpgradeDismissed != "5000|movie" || v.UploadJobID != "upload_1" || v.Message != "keep" {
		t.Fatalf("dismissal changed more than the dismissal: %+v %v", v, err)
	}
	if _, err := s.SetUpgradeDismissed("a", "x", "Another offer", offer.Size); !errors.Is(err, ErrCandidateBusy) {
		t.Fatalf("dismissed a different offer: %v", err)
	}
	if v, err = s.SetUpgradeDismissed("a", "", offer.Title, offer.Size); err != nil || v.UpgradeDismissed != "" {
		t.Fatalf("restore: %+v %v", v, err)
	}

	if err := s.QueueMonitorCandidate("b", time.Now()); !errors.Is(err, ErrCandidateBusy) {
		t.Fatalf("queued a candidate being checked: %v", err)
	}
	if err := s.QueueMonitorCandidate("a", time.Now()); err != nil {
		t.Fatal(err)
	}
	if v, _ = s.MonitorCandidate("a"); v.Status != "queued" || v.UploadJobID != "upload_1" {
		t.Fatalf("queue lost fields: %+v", v)
	}

	// A check's save keeps what changed while it ran.
	check := models.MonitorCandidate{ID: "a", MonitorID: "m", Status: "present", NextCheckAt: time.Now().Add(time.Hour)}
	if err := s.FinishMonitorCheck(check, func(stored models.MonitorCandidate, v *models.MonitorCandidate) { v.UploadJobID = stored.UploadJobID }); err != nil {
		t.Fatal(err)
	}
	if v, _ = s.MonitorCandidate("a"); v.Status != "present" || v.UploadJobID != "upload_1" {
		t.Fatalf("check save: %+v", v)
	}
	if err := s.FinishMonitorCheck(models.MonitorCandidate{ID: "gone"}, func(models.MonitorCandidate, *models.MonitorCandidate) {}); err != nil {
		t.Fatalf("saving a deleted candidate: %v", err)
	}
}

func TestRequeueReachesCandidatesBeingChecked(t *testing.T) {
	s := openTest(t)
	old := time.Now().Add(-time.Hour)
	addCandidate(t, s, models.MonitorCandidate{ID: "a", Status: "present", NextCheckAt: old.Add(48 * time.Hour)})
	addCandidate(t, s, models.MonitorCandidate{ID: "b", Status: "checking", NextCheckAt: old})
	now := time.Now()
	n, err := s.RequeuePolicyCandidates(2, now)
	if err != nil || n != 2 {
		t.Fatalf("requeued %d: %v", n, err)
	}
	a, _ := s.MonitorCandidate("a")
	b, _ := s.MonitorCandidate("b")
	if a.Status != "queued" || b.Status != "checking" || !b.NextCheckAt.Equal(now.UTC()) {
		t.Fatalf("requeue: %+v %+v", a, b)
	}
}

func TestUpgradeCandidatesFilterInSQL(t *testing.T) {
	s := openTest(t)
	addCandidate(t, s, models.MonitorCandidate{ID: "a", Upgrade: "upgrade", UpgradeOffer: &models.PolicyTargetResult{Title: "x", Size: 1}, NextCheckAt: time.Now()})
	addCandidate(t, s, models.MonitorCandidate{ID: "b", NextCheckAt: time.Now()})
	addCandidate(t, s, models.MonitorCandidate{ID: "c", Upgrade: "possible", NextCheckAt: time.Now()})
	for monitor, want := range map[string]int{"": 1, "m": 1, "other": 0} {
		items, err := s.UpgradeCandidates(monitor)
		if err != nil || len(items) != want || (want == 1 && items[0].ID != "a") {
			t.Fatalf("monitor %q: %+v %v", monitor, items, err)
		}
	}
}

func TestFamiliesStoreRawOnce(t *testing.T) {
	s := openTest(t)
	j := models.Job{ID: "j", Status: "completed", StartedAt: time.Now()}
	if err := s.SaveJob(j, "k"); err != nil {
		t.Fatal(err)
	}
	r := models.Result{ID: "r", Title: "x", Raw: json.RawMessage(`{"marker":"raw-response"}`), ParserOutput: json.RawMessage(`{"parser":1}`)}
	familyResult := r
	familyResult.Confidence = "EXACT"
	if err := s.SaveAnalysis(j, []models.Result{r}, []models.Family{{ID: "f", Results: []models.Result{familyResult}}}); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := s.DB.QueryRow("SELECT payload FROM release_families WHERE search_id='j'").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, "raw-response") {
		t.Fatal("family payload repeats the raw response")
	}
	families, err := s.Families("j")
	if err != nil || len(families) != 1 || string(families[0].Results[0].Raw) != `{"marker":"raw-response"}` || string(families[0].Results[0].ParserOutput) != `{"parser":1}` || families[0].Results[0].Confidence != "EXACT" {
		t.Fatalf("family results not restored: %+v %v", families, err)
	}
}

func TestPresetDeleteReportsMissing(t *testing.T) {
	s := openTest(t)
	if err := s.SavePreset(models.IndexerPreset{ID: "p", Name: "P", IndexerIDs: []int{1}}); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.DeletePreset("p"); err != nil || !ok {
		t.Fatalf("delete: %v %v", ok, err)
	}
	if ok, err := s.DeletePreset("p"); err != nil || ok {
		t.Fatalf("second delete: %v %v", ok, err)
	}
}

func TestUploadDispatchAfterAndPrune(t *testing.T) {
	s := openTest(t)
	now := time.Now().UTC()
	recent, old, retry := now.Add(-10*time.Second), now.Add(-time.Hour), now.Add(5*time.Minute)
	for _, j := range []models.UploadJob{
		{ID: "recent", SearchID: "s1", ResultID: "1", Status: "finished", AssistantURL: "http://ua", SubmittedAt: &recent, CreatedAt: now.Add(-72 * time.Hour)},
		{ID: "old", SearchID: "s2", ResultID: "2", Status: "failed", AssistantURL: "http://ua", SubmittedAt: &old, CreatedAt: now.Add(-72 * time.Hour)},
		{ID: "other", SearchID: "s3", ResultID: "3", Status: "finished", AssistantURL: "http://other", SubmittedAt: &recent, CreatedAt: now},
		{ID: "waiting", SearchID: "s4", ResultID: "4", Status: "downloading", AssistantURL: "http://ua", NextAttemptAt: &retry, CreatedAt: now.Add(-72 * time.Hour)},
	} {
		if err := s.CreateUpload(j); err != nil {
			t.Fatal(err)
		}
	}
	after, err := s.UploadDispatchAfter("http://ua", time.Minute)
	if err != nil || !after.Equal(retry) {
		t.Fatalf("dispatch after: %v %v", after, err)
	}
	after, err = s.UploadDispatchAfter("http://other", time.Minute)
	if err != nil || !after.Equal(recent.Add(time.Minute)) {
		t.Fatalf("other assistant: %v %v", after, err)
	}
	if err := s.SaveMetadata("stale", false, map[string]string{}, json.RawMessage(`{}`), now, now.Add(-100*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveMetadata("fresh", false, map[string]string{}, json.RawMessage(`{}`), now, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.Prune(context.Background(), now, 24*time.Hour, 0); err != nil {
		t.Fatal(err)
	}
	for id, kept := range map[string]bool{"recent": false, "old": false, "other": true, "waiting": true} {
		if _, err := s.Upload(id); (err == nil) != kept {
			t.Fatalf("upload %s kept=%v: %v", id, kept, err)
		}
	}
	var out map[string]string
	if _, err := s.Metadata("stale", false, &out); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("long-expired metadata kept: %v", err)
	}
	if _, err := s.Metadata("fresh", false, &out); err != nil {
		t.Fatalf("recently expired metadata removed: %v", err)
	}
}

func TestUploadsFilterByStatusAndCountEachStatus(t *testing.T) {
	s := openTest(t)
	now := time.Now().UTC()
	statuses := []string{"downloading", "finished", "failed", "downloading", "uploading", "cancelled", "downloading"}
	for i, status := range statuses {
		id := fmt.Sprint("job", i)
		if err := s.CreateUpload(models.UploadJob{ID: id, SearchID: "s" + id, ResultID: id, Status: status, CreatedAt: now.Add(time.Duration(i) * time.Minute)}); err != nil {
			t.Fatal(err)
		}
	}
	items, total, counts, err := s.UploadQueue(2, 0, []string{"downloading", "uploading", "downloading"})
	if err != nil || total != 4 || len(items) != 2 || items[0].ID != "job6" || items[1].ID != "job4" {
		t.Fatalf("active page: %+v %d %v", items, total, err)
	}
	if items, _, _, _ = s.UploadQueue(2, 2, []string{"downloading", "uploading"}); len(items) != 2 || items[0].ID != "job3" || items[1].ID != "job0" {
		t.Fatalf("second active page: %+v", items)
	}
	want := map[string]int{"adding": 0, "downloading": 3, "uploading": 1, "needs_review": 0, "failed": 1, "finished": 1, "cancelled": 1}
	if fmt.Sprint(counts) != fmt.Sprint(want) {
		t.Fatalf("status counts: %v", counts)
	}
	if items, total, _, err = s.UploadQueue(20, 0, nil); err != nil || total != len(statuses) || len(items) != len(statuses) {
		t.Fatalf("unfiltered: %d %d %v", len(items), total, err)
	}
	if items, total, _, err = s.UploadQueue(20, 0, []string{"needs_review"}); err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("empty filter: %+v %d %v", items, total, err)
	}
}

func TestTorrentIdentityExpiry(t *testing.T) {
	s := openTest(t)
	now := time.Now()
	if err := s.SaveTorrentIdentity(models.TorrentIdentity{Hash: strings.Repeat("A", 40), Source: models.IdentityArrParse, ExpiresAt: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TorrentIdentity(strings.Repeat("a", 40), now); err != nil {
		t.Fatalf("unexpired identity missing: %v", err)
	}
	if _, err := s.TorrentIdentity(strings.Repeat("a", 40), now.Add(2*time.Minute)); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expired identity returned: %v", err)
	}
	if err := s.PruneTorrentIdentities(now.Add(2 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.DB.QueryRow("SELECT count(*) FROM torrent_identities").Scan(&n); err != nil || n != 0 {
		t.Fatalf("expired identity kept: %d %v", n, err)
	}
}

func TestConnectionPragmas(t *testing.T) {
	s := openTest(t)
	var mode string
	var keys, timeout int
	if err := s.DB.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("journal mode %q: %v", mode, err)
	}
	if err := s.DB.QueryRow("PRAGMA foreign_keys").Scan(&keys); err != nil || keys != 1 {
		t.Fatalf("foreign keys %d: %v", keys, err)
	}
	if err := s.DB.QueryRow("PRAGMA busy_timeout").Scan(&timeout); err != nil || timeout != 5000 {
		t.Fatalf("busy timeout %d: %v", timeout, err)
	}
}
