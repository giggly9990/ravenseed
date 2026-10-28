package search

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"ravenseed/internal/config"
	"ravenseed/internal/database"
	"ravenseed/internal/models"
	"ravenseed/internal/policy"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestMonitorExistingTorrentCandidateAndUpload(t *testing.T) {
	const name = "The.Matrix.1999.1080p.WEB-DL.DDP5.1.H.264-NTb"
	hash := strings.Repeat("a", 40)
	var present atomic.Bool
	var fail atomic.Bool
	var searchCalls atomic.Int32
	var uploadCalls atomic.Int32
	ua := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ua-secret" {
			t.Error("missing UA token")
			w.WriteHeader(401)
			return
		}
		if r.URL.Path == "/api/browse_roots" {
			_, _ = w.Write([]byte(`{"success":true}`))
			return
		}
		if r.URL.Path != "/api/execute" {
			w.WriteHeader(404)
			return
		}
		uploadCalls.Add(1)
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["path"] != "/data/movie.mkv" || !strings.Contains(body["args"], "--trackers AITHER") {
			t.Errorf("bad UA submission: %v", body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"exit\",\"code\":0}\n\n"))
	}))
	defer ua.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "key" {
			t.Error("missing qui API key")
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/api/instances":
			_ = json.NewEncoder(w).Encode([]models.QuiInstance{{ID: 7, Connected: true, IsActive: true}})
		case "/api/instances/7/torrents":
			_ = json.NewEncoder(w).Encode(map[string]any{"torrents": []any{map[string]any{"hash": hash, "name": name, "tracker": "https://a.example/announce?key=secret", "total_size": 1000, "progress": 1, "amount_left": 0, "state": "stalledUP", "content_path": "/downloads/movie.mkv"}}, "total": 1})
		case "/api/instances/7/torrents/" + hash + "/files":
			_ = json.NewEncoder(w).Encode([]any{map[string]any{"size": 1000, "progress": 1}})
		case "/api/torznab/indexers":
			_ = json.NewEncoder(w).Encode([]models.Indexer{{ID: 1, Name: "A", Enabled: true}, {ID: 2, Name: "B", Enabled: true}})
		case "/api/torznab/search":
			searchCalls.Add(1)
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			// An empty title+year search is retried without the year.
			if body["cache_mode"] != "bypass" || (body["query"] != "The Matrix 1999" && (body["query"] != "The Matrix" || body["year"] != nil)) {
				t.Errorf("bad target query: %v", body)
			}
			if fail.Load() {
				_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}, "total": 0, "partial": true})
				return
			}
			rows := []any{}
			if present.Load() {
				rows = append(rows, map[string]any{"indexer_id": 2, "title": name, "size": 1000})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"results": rows, "total": len(rows)})
		default:
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	store, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	s, err := New(store, config.Config{QuiURL: upstream.URL, APIKey: "key", Concurrency: 1, MaxJobs: 1, SearchTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	hosts, err := s.TrackerHosts(ctx)
	if err != nil || len(hosts) != 1 || hosts[0].Host != "a.example" {
		t.Fatalf("hosts: %v %v", hosts, err)
	}
	m, err := s.CreateMonitor(ctx, 1, 2, "a.example", 24, []int{7})
	if err != nil {
		t.Fatal(err)
	}
	items, err := store.MonitorCandidates(m.ID)
	if err != nil || len(items) != 1 || items[0].Status != "queued" {
		t.Fatalf("candidates: %v %v", items, err)
	}
	if _, err := s.CreateMonitor(ctx, 1, 2, "a.example", 24, []int{7}); err == nil {
		t.Fatal("duplicate monitor accepted")
	}
	if _, err := s.CreateMonitor(ctx, 1, 1, "a.example", 24, []int{7}); err == nil {
		t.Fatal("same tracker accepted")
	}
	paused, err := store.SetMonitorEnabled(m.ID, false)
	if err != nil || paused.Enabled {
		t.Fatalf("pause failed: %+v %v", paused, err)
	}
	if _, err := store.DueMonitorCandidate(time.Now()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("paused monitor still has due work: %v", err)
	}
	if claimed, err := store.ClaimMonitorCandidate(items[0].ID); err != nil || claimed {
		t.Fatalf("paused candidate was claimed: %v %v", claimed, err)
	}
	if err := s.ScanMonitor(ctx, m.ID); !errors.Is(err, ErrMonitorPaused) {
		t.Fatalf("paused inventory scan accepted: %v", err)
	}
	if err := s.QueueMonitorCheck(items[0].ID); !errors.Is(err, ErrMonitorPaused) {
		t.Fatalf("paused manual check accepted: %v", err)
	}
	if err := store.InsertMonitorCandidate(models.MonitorCandidate{ID: "paused-new", MonitorID: m.ID, InstanceID: 7, Hash: strings.Repeat("b", 40), Status: "queued", NextCheckAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if pausedItems, err := store.MonitorCandidates(m.ID); err != nil || len(pausedItems) != 1 {
		t.Fatalf("paused monitor discovered a new torrent: %v %v", pausedItems, err)
	}
	m.LastError = "stale scan result"
	if err := store.SaveMonitorScan(m); err != nil {
		t.Fatal(err)
	}
	stillPaused, err := store.Monitor(m.ID)
	if err != nil || stillPaused.Enabled {
		t.Fatalf("scan result reenabled monitor: %+v %v", stillPaused, err)
	}
	resumed, err := store.SetMonitorEnabled(m.ID, true)
	if err != nil || !resumed.Enabled {
		t.Fatalf("resume failed: %+v %v", resumed, err)
	}
	if _, err := store.DueMonitorCandidate(time.Now()); err != nil {
		t.Fatalf("due work did not resume: %v", err)
	}
	claimed, err := store.ClaimMonitorCandidate(items[0].ID)
	if err != nil || !claimed {
		t.Fatalf("claim: %v %v", claimed, err)
	}
	checking, err := store.MonitorCandidate(items[0].ID)
	if err != nil || checking.Status != "checking" {
		t.Fatalf("claim payload: %v %v", checking.Status, err)
	}
	s.checkMonitorCandidate(ctx, items[0])
	v, _ := store.MonitorCandidate(items[0].ID)
	if v.Status != "candidate" || v.LastCheckedAt == nil {
		t.Fatalf("missing release should be candidate: %+v", v)
	}
	if searchCalls.Load() != 2 || v.TargetQuery != "The Matrix 1999 · The Matrix" {
		t.Fatalf("expected a title+year search and a title-only retry: calls=%d query=%q", searchCalls.Load(), v.TargetQuery)
	}
	if _, err := s.QueueExistingUpload(ctx, v.ID, models.MonitorUploadRequest{InstanceID: 7}); err == nil {
		t.Fatal("upload without UA settings accepted")
	}
	present.Store(true)
	s.checkMonitorCandidate(ctx, v)
	v, _ = store.MonitorCandidate(v.ID)
	if v.Status != "present" {
		t.Fatalf("matching release should be present: %+v", v)
	}
	fail.Store(true)
	s.checkMonitorCandidate(ctx, v)
	v, _ = store.MonitorCandidate(v.ID)
	if v.Status != "error" || v.NextCheckAt.After(time.Now().Add(2*time.Hour)) {
		t.Fatalf("partial result must not become candidate: %+v", v)
	}
	fail.Store(false)
	present.Store(false)
	s.checkMonitorCandidate(ctx, v)
	v, _ = store.MonitorCandidate(v.ID)
	if v.Status != "candidate" {
		t.Fatalf("candidate did not recover: %+v", v)
	}
	configured := policy.Default(2)
	configured.Enabled = true
	webSlot := configured.Slots["WEB_DL"]
	webSlot.Provider = false
	configured.Slots["WEB_DL"] = webSlot
	configured, err = store.SaveIndexerPolicy(configured)
	if err != nil {
		t.Fatal(err)
	}
	s.checkMonitorCandidate(ctx, v)
	v, _ = store.MonitorCandidate(v.ID)
	if v.Status != "candidate" || v.Policy == nil || v.Policy.Classification != models.PolicyMissingSlot || v.Policy.PolicySnapshotID != configured.SnapshotID {
		t.Fatalf("policy did not classify monitor candidate: %+v", v)
	}
	if err := s.ConfigureUploads(ctx, models.UploadSettings{Enabled: true, URL: ua.URL, Token: "ua-secret", Trackers: []models.UploadTracker{{IndexerID: 2, Code: "AITHER"}}, Paths: []models.UploadPath{{InstanceID: 7, From: "/downloads", To: "/data"}}}, true); err != nil {
		t.Fatal(err)
	}
	// A candidate on an instance the monitor no longer scans is not queued.
	if _, err := store.SetMonitorInstances(m.ID, []int{8}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.QueueExistingUpload(ctx, v.ID, models.MonitorUploadRequest{InstanceID: 7}); err == nil || !strings.Contains(err.Error(), "no longer scans") {
		t.Fatalf("upload from a dropped instance: %v", err)
	}
	if _, err := store.SetMonitorInstances(m.ID, []int{7}, nil); err != nil {
		t.Fatal(err)
	}
	job, err := s.QueueExistingUpload(ctx, v.ID, models.MonitorUploadRequest{InstanceID: 7, TMDbID: 603})
	if err != nil {
		t.Fatal(err)
	}
	if job.Tag != "" || job.Hash != hash || job.Media.TMDbID != 603 {
		t.Fatalf("existing torrent snapshot wrong: %+v", job)
	}
	if _, err := s.QueueExistingUpload(ctx, v.ID, models.MonitorUploadRequest{InstanceID: 7}); err == nil {
		t.Fatal("duplicate submission accepted")
	}
	s.ProcessUpload(ctx, job.ID)
	finished, err := store.Upload(job.ID)
	if err != nil || finished.Status != "finished" || uploadCalls.Load() != 1 {
		t.Fatalf("UA did not run once: %+v %v calls=%d", finished, err, uploadCalls.Load())
	}
}

func TestMonitorPresenceWithIndexerPolicies(t *testing.T) {
	for _, tc := range []struct {
		name, source, target, query, status string
		size, targetSize                    int64
		partial                             bool
		classification                      models.CandidateClassification
	}{
		{"dual audio annotation", "Vinland Saga S01 1080p NF WEB-DL DDP2 0 H 264-playWEB", "Vinland Saga S01 1080p NF WEB-DL Dual-Audio DD+ 2.0 H.264-playWEB", "Vinland Saga", "present", 28851404800, 28851404800, false, models.PolicyEquivalent},
		{"occupied is not a speculative trump", "Vinland Saga S01 1080p NF WEB-DL DDP2 0 H 264-playWEB", "Vinland Saga S01 1080p NF WEB-DL Dual-Audio DD+ 2.0 H.264-playWEB", "Vinland Saga", "occupied", 28851404800, 28851404801, false, models.PolicyNotEligible},
		{"reported identical tracker titles", "FBI: International S04 1080p AMZN WEB-DL DD+ 5.1 H.264-NTb", "FBI: International S04 1080p AMZN WEB-DL DD+ 5.1 H.264-NTb", "FBI: International", "present", 54645880080, 54645880080, false, models.PolicyEquivalent},
		{"title punctuation", "FBI.International.S04.1080p.AMZN.WEB-DL.DDP5.1.H.264-NTb", "FBI: International S04 1080p AMZN WEB-DL DD+ 5.1 H.264-NTb", "FBI International", "present", 54645880080, 54645880080, false, models.PolicyEquivalent},
		{"another season cannot match", "FBI.International.S04.1080p.AMZN.WEB-DL.DDP5.1.H.264-NTb", "FBI: International S03 1080p AMZN WEB-DL DDP5.1 H.264-NTb", "FBI International", "candidate", 54645880080, 54645880080, false, models.PolicyMissingSlot},
		{"unnamed source resolution", "Scrubs.S06.DSNP.WEB-DL.AAC2.0.H.264-playWEB", "Scrubs S06 1080p DSNP WEB-DL AAC 2.0 H.264-playWEB", "Scrubs", "present", 11417325401, 11417325401, false, models.PolicyEquivalent},
		{"rounded target size holds the slot", "Scrubs.S06.1080p.DSNP.WEB-DL.AAC2.0.H.264-playWEB", "Scrubs S06 1080p DSNP WEB-DL AAC 2.0 H.264-playWEB", "Scrubs", "occupied", 11417325401, 11413953167, false, models.PolicyNotEligible},
		{"season pack holds a single episode's season", "Show.S01E03.1080p.AMZN.WEB-DL.DDP5.1.H.264-GRP", "Show S01 1080p AMZN WEB-DL DDP5.1 H.264-GRP", "Show", "occupied", 1000, 30000, false, models.PolicyNotEligible},
		{"partial search stays error", "FBI.International.S04.1080p.AMZN.WEB-DL.DDP5.1.H.264-NTb", "FBI: International S04 1080p AMZN WEB-DL DD+ 5.1 H.264-NTb", "FBI International", "error", 54645880080, 54645880080, true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hash := strings.Repeat("a", 40)
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/instances/7/torrents":
					_ = json.NewEncoder(w).Encode(map[string]any{"torrents": []any{map[string]any{"hash": hash, "name": tc.source, "total_size": tc.size, "tracker": "https://source.example/announce"}}})
				case "/api/torznab/indexers":
					_ = json.NewEncoder(w).Encode([]models.Indexer{{ID: 2, Name: "Target", Enabled: true}})
				case "/api/torznab/search":
					calls.Add(1)
					var body map[string]any
					_ = json.NewDecoder(r.Body).Decode(&body)
					if body["query"] != tc.query || body["season"] != nil || body["episode"] != nil {
						t.Errorf("monitor query should search series title and scope locally: %v", body)
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"total": 1, "partial": tc.partial, "results": []any{map[string]any{"title": tc.target, "size": tc.targetSize, "info_url": "https://target.example/torrents/1", "indexer_id": 2}}})
				default:
					w.WriteHeader(404)
				}
			}))
			defer upstream.Close()
			store, err := database.Open(filepath.Join(t.TempDir(), "monitor.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.DB.Close()
			s, err := New(store, config.Config{QuiURL: upstream.URL, APIKey: "key", Concurrency: 1, MaxJobs: 1, SearchTimeout: time.Second})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			monitor := models.Monitor{ID: "m", Enabled: true, SourceIndexerID: 1, TargetIndexerID: 2, SourceTrackerHost: "source.example", QuiURL: upstream.URL, IntervalHours: 24, CreatedAt: time.Now()}
			if err := store.CreateMonitor(monitor); err != nil {
				t.Fatal(err)
			}
			candidate := models.MonitorCandidate{ID: "c", MonitorID: "m", InstanceID: 7, Hash: hash, Name: tc.source, Size: tc.size, Status: "queued", NextCheckAt: time.Now()}
			if err := store.InsertMonitorCandidate(candidate); err != nil {
				t.Fatal(err)
			}
			configured := policy.Presets()[2].Policy
			configured.IndexerID = 2
			if _, err := store.SaveIndexerPolicy(configured); err != nil {
				t.Fatal(err)
			}
			s.checkMonitorCandidate(context.Background(), candidate)
			got, err := store.MonitorCandidate("c")
			if err != nil || got.Status != tc.status {
				t.Fatalf("status %s, want %s: %+v %v", got.Status, tc.status, got, err)
			}
			if calls.Load() != 1 || got.TargetQuery != tc.query || got.TargetResultCount != 1 {
				t.Fatalf("query diagnostics or cadence wrong: calls=%d %+v", calls.Load(), got)
			}
			if strings.HasPrefix(tc.name, "season pack") && (got.Upgrade != "upgrade" || got.UpgradeOffer == nil || got.UpgradeOffer.Upgrade[0].Type != "tv_structure") {
				t.Fatalf("season pack not offered as an upgrade over the single episode: %s %+v", got.Upgrade, got.UpgradeOffer)
			}
			if tc.classification != "" && (got.Policy == nil || got.Policy.Classification != tc.classification) {
				t.Fatalf("policy classification: %+v", got.Policy)
			}
			if got.Status == "present" && (got.MatchCount != 1 || len(got.Policy.TargetResults) != 1 || got.Policy.TargetResults[0].Title != tc.target) {
				t.Fatalf("existing target evidence missing: %+v", got)
			}
			if got.Status == "occupied" {
				if _, err := s.QueueExistingUpload(context.Background(), got.ID, models.MonitorUploadRequest{InstanceID: 7}); err == nil || !strings.Contains(err.Error(), "candidate needs") {
					t.Fatalf("occupied slot did not fail the candidate gate: %v", err)
				}
			}
		})
	}
}

func TestCancelledScanIsNotAMonitorError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]models.QuiInstance{{ID: 7, Connected: true, IsActive: true}})
	}))
	defer upstream.Close()
	store, err := database.Open(filepath.Join(t.TempDir(), "scan.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	s, err := New(store, config.Config{QuiURL: upstream.URL, APIKey: "key", Concurrency: 1, MaxJobs: 1, SearchTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := store.CreateMonitor(models.Monitor{ID: "m", Enabled: true, SourceIndexerID: 1, TargetIndexerID: 2, SourceTrackerHost: "a.example", QuiURL: upstream.URL, IntervalHours: 24, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.ScanMonitor(ctx, "m"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation: %v", err)
	}
	s.scanAllMonitors(ctx)
	if m, _ := store.Monitor("m"); m.LastError != "" {
		t.Fatalf("cancellation recorded as a monitor error: %q", m.LastError)
	}
}

func TestMonitorScansOnlySelectedInstances(t *testing.T) {
	torrent := func(hash, tracker string) map[string]any {
		return map[string]any{"hash": hash, "name": "The.Matrix.1999.1080p.WEB-DL.DDP5.1.H.264-NTb", "tracker": tracker, "total_size": 1000}
	}
	var offline8 atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/instances":
			_ = json.NewEncoder(w).Encode([]models.QuiInstance{{ID: 7, Connected: true, IsActive: true}, {ID: 8, Connected: !offline8.Load(), IsActive: true}, {ID: 9, Connected: false, IsActive: true}})
		case "/api/instances/7/torrents":
			_ = json.NewEncoder(w).Encode(map[string]any{"torrents": []any{torrent(strings.Repeat("a", 40), "https://a.example/announce")}, "total": 1})
		case "/api/instances/8/torrents":
			_ = json.NewEncoder(w).Encode(map[string]any{"torrents": []any{torrent(strings.Repeat("b", 40), "https://a.example/announce"), torrent(strings.Repeat("c", 40), "https://c.example/announce")}, "total": 2})
		case "/api/torznab/indexers":
			_ = json.NewEncoder(w).Encode([]models.Indexer{{ID: 1, Name: "A", Enabled: true}, {ID: 2, Name: "B", Enabled: true}})
		default:
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	store, err := database.Open(filepath.Join(t.TempDir(), "instances.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	s, err := New(store, config.Config{QuiURL: upstream.URL, APIKey: "key", Concurrency: 1, MaxJobs: 1, SearchTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	hosts, err := s.TrackerHosts(ctx)
	if err != nil || len(hosts) != 2 || hosts[0].Host != "a.example" || hosts[0].Count != 2 || hosts[0].InstanceCounts[7] != 1 || hosts[0].InstanceCounts[8] != 1 || hosts[1].InstanceCounts[7] != 0 {
		t.Fatalf("hosts: %+v %v", hosts, err)
	}
	for name, instances := range map[string][]int{"none": nil, "disconnected": {9}, "unknown": {42}} {
		if _, err := s.CreateMonitor(ctx, 1, 2, "a.example", 24, instances); err == nil {
			t.Fatalf("%s instance selection accepted", name)
		}
	}
	if _, err := s.CreateMonitor(ctx, 1, 2, "c.example", 24, []int{7}); err == nil {
		t.Fatal("host missing from the chosen instances accepted")
	}
	m, err := s.CreateMonitor(ctx, 1, 2, "a.example", 24, []int{7, 7})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.InstanceIDs) != 1 || m.InstanceIDs[0] != 7 {
		t.Fatalf("instances: %v", m.InstanceIDs)
	}
	if err := s.ScanMonitor(ctx, m.ID); err != nil {
		t.Fatal(err)
	}
	items, err := store.MonitorCandidates(m.ID)
	if err != nil || len(items) != 1 || items[0].InstanceID != 7 {
		t.Fatalf("candidates: %+v %v", items, err)
	}

	// Monitors saved before instance selection keep scanning every instance.
	if err := store.CreateMonitor(models.Monitor{ID: "legacy", Enabled: true, SourceIndexerID: 2, TargetIndexerID: 1, SourceTrackerHost: "a.example", QuiURL: upstream.URL, IntervalHours: 24, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := s.ScanMonitor(ctx, "legacy"); err != nil {
		t.Fatal(err)
	}
	if items, err := store.MonitorCandidates("legacy"); err != nil || len(items) != 2 {
		t.Fatalf("legacy candidates: %+v %v", items, err)
	}

	// Edits are validated like creation, scan new instances at once and queue
	// candidates on dropped ones, whose next check marks them gone.
	for name, instances := range map[string][]int{"disconnected": {9}, "unknown": {42}} {
		if _, err := s.SetMonitorInstances(ctx, m.ID, instances, nil); err == nil || !strings.Contains(err.Error(), "instance") {
			t.Fatalf("%s instance edit accepted: %v", name, err)
		}
	}
	if _, err := s.SetMonitorInstances(ctx, "missing", []int{8}, nil); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing monitor: %v", err)
	}
	dropped := items[0]
	dropped.Status = "candidate"
	if err := store.SaveMonitorCandidate(dropped); err != nil {
		t.Fatal(err)
	}
	if m, err = s.SetMonitorInstances(ctx, m.ID, []int{8, 8}, nil); err != nil || !slices.Equal(m.InstanceIDs, []int{8}) || !m.Enabled {
		t.Fatalf("edit: %+v %v", m, err)
	}
	if items, err := store.MonitorCandidates(m.ID); err != nil || len(items) != 2 {
		t.Fatalf("candidates after edit: %+v %v", items, err)
	}
	if dropped, err = store.MonitorCandidate(dropped.ID); err != nil || dropped.Status != "queued" {
		t.Fatalf("dropped candidate not queued: %+v %v", dropped, err)
	}
	s.checkMonitorCandidate(ctx, dropped)
	if got, err := store.MonitorCandidate(dropped.ID); err != nil || got.Status != "gone" || !strings.Contains(got.Message, "no longer scans") {
		t.Fatalf("dropped candidate: %+v %v", got, err)
	}
	// Re-adding an instance queues its gone candidates for a fresh check.
	if m, err = s.SetMonitorInstances(ctx, m.ID, []int{7, 8}, nil); err != nil || !slices.Equal(m.InstanceIDs, []int{7, 8}) {
		t.Fatalf("re-add: %+v %v", m, err)
	}
	if got, err := store.MonitorCandidate(dropped.ID); err != nil || got.Status != "queued" {
		t.Fatalf("re-added candidate not queued: %+v %v", got, err)
	}

	// A covered instance is kept while offline; an added one must be online.
	offline8.Store(true)
	if _, err := s.SetMonitorInstances(ctx, m.ID, []int{8, 9}, nil); err == nil || !strings.Contains(err.Error(), "instance 9") {
		t.Fatalf("offline added instance accepted: %v", err)
	}
	if m, err = s.SetMonitorInstances(ctx, m.ID, []int{8}, nil); err != nil || !slices.Equal(m.InstanceIDs, []int{8}) || !strings.Contains(m.LastError, "none of this monitor's qui instances") {
		t.Fatalf("kept offline instance: %+v %v", m, err)
	}
	if items, err := store.MonitorCandidates(m.ID); err != nil || slices.ContainsFunc(items, func(v models.MonitorCandidate) bool { return v.InstanceID == 8 && v.Status == "gone" }) {
		t.Fatalf("kept instance's candidates dropped: %+v %v", items, err)
	}
	// An edit clears the scan error, which a paused monitor would keep.
	paused := false
	if m, err = s.SetMonitorInstances(ctx, m.ID, []int{8}, &paused); err != nil || m.Enabled || m.LastError != "" {
		t.Fatalf("paused edit kept the scan error: %+v %v", m, err)
	}
	// A legacy monitor keeps the offline instances it found torrents on.
	if legacy, err := s.SetMonitorInstances(ctx, "legacy", []int{7, 8}, nil); err != nil || !slices.Equal(legacy.InstanceIDs, []int{7, 8}) {
		t.Fatalf("legacy kept offline instance: %+v %v", legacy, err)
	}
	if _, err := s.SetMonitorInstances(ctx, "legacy", []int{9}, nil); err == nil {
		t.Fatal("legacy monitor added an offline instance")
	}
	offline8.Store(false)
	// The enabled flag applies with the selection.
	if legacy, err := s.SetMonitorInstances(ctx, "legacy", []int{7}, &paused); err != nil || legacy.Enabled || !slices.Equal(legacy.InstanceIDs, []int{7}) {
		t.Fatalf("legacy edit: %+v %v", legacy, err)
	}

	// No instances turns a scoped monitor back into an all-instances one.
	if err := store.CreateMonitor(models.Monitor{ID: "scoped", Enabled: true, SourceIndexerID: 2, TargetIndexerID: 3, SourceTrackerHost: "a.example", QuiURL: upstream.URL, IntervalHours: 24, CreatedAt: time.Now(), InstanceIDs: []int{7}}); err != nil {
		t.Fatal(err)
	}
	if err := s.ScanMonitor(ctx, "scoped"); err != nil {
		t.Fatal(err)
	}
	all, err := s.SetMonitorInstances(ctx, "scoped", []int{}, nil)
	if err != nil || all.InstanceIDs != nil || !all.Enabled {
		t.Fatalf("all instances: %+v %v", all, err)
	}
	if err := s.ScanMonitor(ctx, "scoped"); err != nil {
		t.Fatal(err)
	}
	if items, err := store.MonitorCandidates("scoped"); err != nil || len(items) != 2 || items[0].InstanceID == items[1].InstanceID {
		t.Fatalf("all-instances scan: %+v %v", items, err)
	}

	// A scan that began before an instance was dropped adds nothing there.
	stale := models.Monitor{ID: "stale", Enabled: true, SourceIndexerID: 3, TargetIndexerID: 1, SourceTrackerHost: "a.example", QuiURL: upstream.URL, IntervalHours: 24, CreatedAt: time.Now(), InstanceIDs: []int{7, 8}}
	if err := store.CreateMonitor(stale); err != nil {
		t.Fatal(err)
	}
	c, _, _ := s.monitorClient()
	inventory, err := s.inventory(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetMonitorInstances("stale", []int{7}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.saveInventory(stale, inventory); err != nil {
		t.Fatal(err)
	}
	if err := store.InsertMonitorCandidate(models.MonitorCandidate{ID: "phantom", MonitorID: "stale", InstanceID: 8, Hash: strings.Repeat("d", 40), Status: "queued", NextCheckAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if items, err := store.MonitorCandidates("stale"); err != nil || len(items) != 1 || items[0].InstanceID != 7 {
		t.Fatalf("dropped instance got candidates: %+v %v", items, err)
	}
	// Nor does it clear the offline error of an edit onto offline instances.
	if _, err := store.SetMonitorInstances("stale", []int{9}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.saveInventory(stale, inventory); !errors.Is(err, errMonitorOffline) {
		t.Fatalf("stale scan of an offline selection: %v", err)
	}

	// A scoped monitor whose instances are all offline reports it.
	if err := store.CreateMonitor(models.Monitor{ID: "offline", Enabled: true, SourceIndexerID: 1, TargetIndexerID: 2, SourceTrackerHost: "c.example", QuiURL: upstream.URL, IntervalHours: 24, CreatedAt: time.Now(), InstanceIDs: []int{9}}); err != nil {
		t.Fatal(err)
	}
	var upstreamErr UpstreamError
	if err := s.ScanMonitor(ctx, "offline"); !errors.As(err, &upstreamErr) || !strings.Contains(err.Error(), "none of this monitor's qui instances") {
		t.Fatalf("offline scan: %v", err)
	}
	if got, _ := store.Monitor("offline"); got.LastError == "" || got.LastScanAt != nil {
		t.Fatalf("offline scan not recorded: %+v", got)
	}
	if err := store.SaveMonitorScan(models.Monitor{ID: "offline"}); err != nil {
		t.Fatal(err)
	}
	s.scanAllMonitors(ctx)
	if got, _ := store.Monitor("offline"); !strings.Contains(got.LastError, "none of this monitor's qui instances") {
		t.Fatalf("scheduled offline scan not recorded: %+v", got)
	}
}

func TestMonitorPolicyContextKeepsMeasuredBitrate(t *testing.T) {
	store, err := database.Open(filepath.Join(t.TempDir(), "bitrate.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	s, err := New(store, config.Config{QuiURL: "http://127.0.0.1:1", APIKey: "key", Concurrency: 1, MaxJobs: 1, SearchTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// The identity lacks a runtime, so cached TMDb metadata supplies it.
	media := models.Media{Type: "movie", TMDbID: 603}
	if err := store.SaveMetadata(metadataKey(media, s.settings.Metadata.Language), false, models.MediaSnapshot{Media: media, Runtime: 136}, json.RawMessage(`{}`), time.Now(), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	identity := &models.TorrentIdentity{Source: models.IdentityArrHistory, Media: media, VideoMbps: 12}
	configured := models.IndexerPolicy{EncodeBitrateTiers: []models.EncodeBitrateTier{{}}}
	got := s.monitorPolicyContext(context.Background(), configured, models.MonitorCandidate{Media: media, Identity: identity})
	if got.RuntimeMinutes != 136 || got.SourceVideoMbps != 12 {
		t.Fatalf("TMDb fallback context: %+v", got)
	}
}
