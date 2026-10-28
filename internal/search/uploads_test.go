package search

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"ravenseed/internal/config"
	"ravenseed/internal/database"
	"ravenseed/internal/models"
	"ravenseed/internal/qui"
	"strings"
	"sync"
	"testing"
	"time"
)

type uploadFixture struct {
	mu                    sync.Mutex
	state                 string
	progress              float64
	files                 bool
	calls                 int
	adds, exitCode        int
	events                string
	session               string
	disconnected          bool
	tag, args, uploadPath string
	s                     *Service
	ua                    string
	holdExecute           <-chan struct{}
	holdPoll              <-chan struct{}
	holdAfterEvents       <-chan struct{}
	polled                chan struct{}
	uniqueHashes          bool
	active, maximum       int
	callTimes             []time.Time
	browseCalls           int
	rateLimitRemaining    int
	retryAfter            string
}

func newUploadFixture(t *testing.T, concurrency ...int) *uploadFixture {
	t.Helper()
	f := &uploadFixture{state: "downloading", progress: .5, files: false}
	u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer ua-secret" {
			t.Error("UA auth lost")
			w.WriteHeader(401)
			return
		}
		if r.URL.Path == "/api/browse_roots" {
			f.browseCalls++
			_, _ = w.Write([]byte(`{"success":true,"items":[]}`))
			return
		}
		if r.URL.Path != "/api/execute" {
			t.Error("wrong UA endpoint")
			return
		}
		f.calls++
		f.callTimes = append(f.callTimes, time.Now())
		if f.rateLimitRemaining > 0 {
			f.rateLimitRemaining--
			if f.retryAfter != "" {
				w.Header().Set("Retry-After", f.retryAfter)
			}
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		f.active++
		if f.active > f.maximum {
			f.maximum = f.active
		}
		defer func() { f.active-- }()
		if hold := f.holdExecute; hold != nil {
			w.Header().Set("Content-Type", "text/event-stream")
			w.(http.Flusher).Flush()
			f.mu.Unlock()
			select {
			case <-hold:
			case <-r.Context().Done():
			}
			f.mu.Lock()
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.args, f.uploadPath, f.session = body["args"], body["path"], body["session_id"]
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, f.events)
		if hold := f.holdAfterEvents; hold != nil {
			w.(http.Flusher).Flush()
			f.mu.Unlock()
			select {
			case <-hold:
			case <-r.Context().Done():
			}
			f.mu.Lock()
		}
		if !f.disconnected {
			_, _ = fmt.Fprintf(w, "data: {\"type\":\"exit\",\"code\":%d}\n\n", f.exitCode)
		}
	}))
	t.Cleanup(u.Close)
	f.ua = u.URL
	q := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Header.Get("X-API-Key") != "qui-secret" {
			t.Error("qui auth lost")
		}
		switch {
		case r.URL.Path == "/api/torznab/indexers":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == "POST":
			f.adds++
			_ = r.ParseMultipartForm(1 << 20)
			f.tag = r.FormValue("tags")
			if r.FormValue("paused") != "false" {
				t.Error("automated download did not start")
			}
			_, _ = w.Write([]byte(`{"added":1,"failed":0}`))
		case strings.HasSuffix(r.URL.Path, "/files"):
			p := 0.5
			if f.files {
				p = 1
			}
			_ = json.NewEncoder(w).Encode([]any{map[string]any{"size": 10, "progress": p}})
		default:
			if hold := f.holdPoll; hold != nil {
				select {
				case f.polled <- struct{}{}:
				default:
				}
				f.mu.Unlock()
				select {
				case <-hold:
				case <-r.Context().Done():
				}
				f.mu.Lock()
			}
			var filter struct {
				Tags []string `json:"tags"`
			}
			_ = json.Unmarshal([]byte(r.URL.Query().Get("filters")), &filter)
			tag := ""
			if len(filter.Tags) == 1 {
				tag = filter.Tags[0]
			}
			hash := strings.Repeat("a", 40)
			if f.uniqueHashes {
				hash = fmt.Sprintf("%x", sha256.Sum256([]byte(tag)))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"torrents": []any{map[string]any{"hash": hash, "tags": tag, "state": f.state, "progress": f.progress, "amount_left": 0, "content_path": "/downloads/Movie/movie.mkv"}}})
		}
	}))
	t.Cleanup(q.Close)
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.DB.Close() })
	workers := 1
	if len(concurrency) > 0 {
		workers = concurrency[0]
	}
	s, err := New(db, config.Config{QuiURL: q.URL, APIKey: "qui-secret", Concurrency: 1, MaxJobs: 1, DefaultCoverage: 2, SearchTimeout: time.Second, UploadConcurrency: workers, UploadSubmissionInterval: time.Nanosecond})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	f.s = s
	err = s.ConfigureUploads(context.Background(), models.UploadSettings{Enabled: true, URL: u.URL, Token: "ua-secret", Trackers: []models.UploadTracker{{IndexerID: 3, Code: "AITHER"}}, Paths: []models.UploadPath{{InstanceID: 7, From: "/downloads", To: "/data"}}}, true)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *uploadFixture) queue(t *testing.T, searchID string) string {
	t.Helper()
	response, err := f.s.QueueUpload(context.Background(), models.Job{ID: searchID, Request: models.SearchRequest{TargetIndexerID: 3, Media: models.Media{Type: "movie", TMDbID: 603}}}, models.Result{ID: "result1", IndexerID: 1, Title: "Movie", DownloadURL: "https://tracker.example/file"}, models.AddTorrentRequest{InstanceID: 7, UploadAfterDownload: true})
	if err != nil {
		t.Fatal(err)
	}
	return response.UploadJobID
}

func TestQueueUploadErrorKinds(t *testing.T) {
	f := newUploadFixture(t)
	job := models.Job{ID: "kinds", Request: models.SearchRequest{TargetIndexerID: 3}}
	row := models.Result{ID: "result", IndexerID: 1, DownloadURL: "https://tracker.example/file"}
	input := models.AddTorrentRequest{InstanceID: 7, UploadAfterDownload: true}
	// A result without a usable link is a rejected input, and queues nothing.
	bad := row
	bad.DownloadURL = "ftp://tracker.example/file"
	var upstreamErr UpstreamError
	var internalErr InternalError
	if _, err := f.s.QueueUpload(context.Background(), job, bad, input); !errors.Is(err, qui.ErrNoDownloadLink) || errors.As(err, &upstreamErr) {
		t.Fatalf("unusable link: %v", err)
	}
	if _, total, _, _ := f.s.Store.UploadQueue(20, 0, nil); total != 0 {
		t.Fatal("unusable link queued a job")
	}
	// A qui timeout stays a timeout.
	expired, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	var hint TimeoutHint
	if _, err := f.s.QueueUpload(expired, job, row, input); !errors.Is(err, context.DeadlineExceeded) || errors.As(err, &upstreamErr) || !errors.As(err, &hint) || !strings.Contains(hint.Hint, "Upload queue") {
		t.Fatalf("timeout: %v", err)
	}
	// A failed save after the add is a server fault.
	if _, err := f.s.Store.DB.Exec("CREATE TRIGGER fail_save BEFORE UPDATE ON upload_jobs BEGIN SELECT RAISE(ABORT,'disk full'); END"); err != nil {
		t.Fatal(err)
	}
	job.ID = "kinds-save"
	if _, err := f.s.QueueUpload(context.Background(), job, row, input); !errors.As(err, &internalErr) || errors.As(err, &upstreamErr) {
		t.Fatalf("save failure: %v", err)
	}
}

func TestUploadWaitsForAllFilesAndSubmitsOnce(t *testing.T) {
	f := newUploadFixture(t)
	id := f.queue(t, "search1")
	f.s.ProcessUpload(context.Background(), id)
	f.mu.Lock()
	f.progress = 1
	f.state = "checkingUP"
	f.mu.Unlock()
	f.s.ProcessUpload(context.Background(), id)
	f.mu.Lock()
	f.state = "stalledUP"
	f.mu.Unlock()
	f.s.ProcessUpload(context.Background(), id)
	f.mu.Lock()
	if f.calls != 0 {
		t.Error("incomplete files uploaded")
	}
	f.files = true
	f.mu.Unlock()
	f.s.ProcessUpload(context.Background(), id)
	f.s.ProcessUpload(context.Background(), id)
	j, _ := f.s.Store.Upload(id)
	if j.Status != "finished" || j.Hash == "" {
		t.Fatalf("job: %+v", j)
	}
	f.mu.Lock()
	if f.calls != 1 || f.uploadPath != "/data/Movie/movie.mkv" || !strings.Contains(f.args, "--trackers AITHER") || strings.Contains(f.args, "--skip-dupe-check") {
		t.Error("incorrect upload submission")
	}
	f.mu.Unlock()
	id2 := f.queue(t, "another-search")
	f.s.ProcessUpload(context.Background(), id2)
	j, _ = f.s.Store.Upload(id2)
	if j.Status != "needs_review" {
		t.Fatal("duplicate hash and target were submitted")
	}
}

func TestUploadSnapshotsPresetAndExecutesSavedFlags(t *testing.T) {
	f := newUploadFixture(t)
	v, _ := f.s.UploadSettings()
	v.FlagPresets = []models.UploadFlagPreset{{ID: "private", Name: "Private moderation", Args: `--anon --modq --description "A quoted title"`}}
	v.DefaultFlagPresetID = "private"
	if err := f.s.ConfigureUploads(context.Background(), v, true); err != nil {
		t.Fatal(err)
	}
	id := f.queue(t, "preset-search")
	v.FlagPresets = []models.UploadFlagPreset{}
	v.DefaultFlagPresetID = ""
	if err := f.s.ConfigureUploads(context.Background(), v, true); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Store.RecoverUploads(); err != nil {
		t.Fatal(err)
	}
	job, _ := f.s.Store.Upload(id)
	if job.FlagPresetName != "Private moderation" || !strings.Contains(job.Args, "--anon --modq") {
		t.Fatal("preset not snapshotted")
	}
	f.mu.Lock()
	f.progress, f.state, f.files = 1, "stalledUP", true
	f.mu.Unlock()
	f.s.ProcessUpload(context.Background(), id)
	job, _ = f.s.Store.Upload(id)
	if job.Status != "finished" {
		t.Fatal("upload failed", job.Message)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls != 1 || !strings.Contains(f.args, "--anon --modq --description 'A quoted title'") || !strings.Contains(f.args, "--trackers AITHER") {
		t.Fatal("preset flags did not reach UA")
	}
}

func TestUploadPresetOverrideAndMissingPreset(t *testing.T) {
	f := newUploadFixture(t)
	v, _ := f.s.UploadSettings()
	v.FlagPresets = []models.UploadFlagPreset{{ID: "anon", Name: "Anonymous", Args: "--anon"}}
	v.DefaultFlagPresetID = "anon"
	if err := f.s.ConfigureUploads(context.Background(), v, true); err != nil {
		t.Fatal(err)
	}
	empty := ""
	input := models.AddTorrentRequest{InstanceID: 7, UploadAfterDownload: true, UploadPresetID: &empty}
	job := models.Job{ID: "no-preset", Request: models.SearchRequest{TargetIndexerID: 3}}
	row := models.Result{ID: "result", IndexerID: 1, DownloadURL: "https://tracker.example/file"}
	out, err := f.s.QueueUpload(context.Background(), job, row, input)
	if err != nil {
		t.Fatal(err)
	}
	queued, _ := f.s.Store.Upload(out.UploadJobID)
	if queued.Args != "" {
		t.Fatal("explicit no preset ignored")
	}
	missing := "deleted"
	input.UploadPresetID = &missing
	job.ID = "missing-preset"
	if _, err := f.s.QueueUpload(context.Background(), job, row, input); err == nil {
		t.Fatal("missing preset accepted")
	}
	_, total, _, _ := f.s.Store.UploadQueue(20, 0, nil)
	if total != 1 {
		t.Fatal("invalid preset queued a download")
	}
}

func TestUploadCancellationAndRecovery(t *testing.T) {
	f := newUploadFixture(t)
	id := f.queue(t, "search1")
	if err := f.s.CancelUpload(id); err != nil {
		t.Fatal(err)
	}
	f.s.ProcessUpload(context.Background(), id)
	j, _ := f.s.Store.Upload(id)
	if j.Status != "cancelled" {
		t.Fatal("cancel lost")
	}
	id2 := f.queue(t, "search2")
	if err := f.s.Store.RecoverUploads(); err != nil {
		t.Fatal(err)
	}
	j, _ = f.s.Store.Upload(id2)
	if j.Status != "downloading" {
		t.Fatal("waiting job did not survive restart")
	}
	j.Status = "uploading"
	_ = f.s.Store.SaveUpload(j)
	_ = f.s.Store.RecoverUploads()
	j, _ = f.s.Store.Upload(id2)
	if j.Status != "needs_review" {
		t.Fatal("interrupted upload not quarantined")
	}
}

func TestUploadOutcomeFollowsTrackerResult(t *testing.T) {
	result := func(status, extra string) string {
		return `data: {"type":"gui","data":{"kind":"tracker","tracker":"AITHER","status":"` + status + `"` + extra + "}}\n\n"
	}
	for _, tc := range []struct {
		name, events, status, message string
		code                          int
	}{
		{"dupe exits zero", result("pending", "") + result("dupe", `,"message":"Possible dupe on this tracker"`), "failed", "Not uploaded to AITHER (possible dupe): Possible dupe on this tracker", 0},
		{"skip exits zero", result("skipped", `,"message":"Check declined: no English audio"`), "failed", "Not uploaded to AITHER (skipped): Check declined: no English audio", 0},
		{"banned group", result("banned", ""), "failed", "Not uploaded to AITHER (release group banned).", 0},
		{"failure exits zero", result("failed", `,"message":"Upload did not complete"`), "failed", "Upload to AITHER failed: Upload did not complete", 0},
		{"upload then error exit", result("success", `,"link":"https://aither.example/torrents/9","torrent_id":"9"`), "finished", "Uploaded to AITHER. Upload-Assistant exited with code 1.", 1},
		{"older UA", "", "finished", "Upload-Assistant finished. Check its WebUI for the tracker outcome; a skipped upload can also exit successfully.", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newUploadFixture(t)
			id := f.queue(t, "search1")
			f.mu.Lock()
			f.progress, f.state, f.files, f.events, f.exitCode = 1, "stalledUP", true, tc.events, tc.code
			f.mu.Unlock()
			f.s.ProcessUpload(context.Background(), id)
			j, _ := f.s.Store.Upload(id)
			if j.Status != tc.status || j.Message != tc.message {
				t.Fatalf("outcome: %s %q", j.Status, j.Message)
			}
			if tc.events != "" && (j.TrackerResult == nil || j.TrackerResult.Status == "pending") {
				t.Fatalf("tracker result not kept: %+v", j.TrackerResult)
			}
			if tc.code == 1 && (j.TrackerResult.Link != "https://aither.example/torrents/9" || j.TrackerResult.TorrentID != "9") {
				t.Fatalf("upload link lost: %+v", j.TrackerResult)
			}
		})
	}
}

func TestUncertainUploadIsNotRetried(t *testing.T) {
	f := newUploadFixture(t)
	id := f.queue(t, "search1")
	f.mu.Lock()
	f.progress, f.state, f.files, f.disconnected = 1, "stalledUP", true, true
	f.mu.Unlock()
	f.s.ProcessUpload(context.Background(), id)
	f.s.ProcessUpload(context.Background(), id)
	j, _ := f.s.Store.Upload(id)
	if j.Status != "needs_review" {
		t.Fatal("uncertain result not held")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls != 1 {
		t.Fatal("upload retried")
	}
}

func TestTrackerResultSurvivesLostStream(t *testing.T) {
	f := newUploadFixture(t)
	id := f.queue(t, "search1")
	f.mu.Lock()
	f.progress, f.state, f.files, f.disconnected = 1, "stalledUP", true, true
	f.events = `data: {"type":"gui","data":{"kind":"tracker","tracker":"AITHER","status":"success","link":"https://aither.example/torrents/9","torrent_id":9}}` + "\n\n"
	f.mu.Unlock()
	f.s.ProcessUpload(context.Background(), id)
	j, _ := f.s.Store.Upload(id)
	if j.Status != "needs_review" || j.TrackerResult == nil || j.TrackerResult.Status != "success" || j.TrackerResult.Link != "https://aither.example/torrents/9" {
		t.Fatalf("tracker result lost: %s %+v", j.Status, j.TrackerResult)
	}
}

func TestPromptAfterStageChangeIsSaved(t *testing.T) {
	f := newUploadFixture(t)
	id := f.queue(t, "search1")
	release := make(chan struct{})
	f.mu.Lock()
	f.progress, f.state, f.files, f.holdAfterEvents = 1, "stalledUP", true, release
	f.events = `data: {"type":"gui","data":{"kind":"stage","key":"dupes","status":"start"}}` + "\n\n" +
		`data: {"type":"gui","data":{"kind":"prompt","action":"open","id":"p1"}}` + "\n\n"
	f.mu.Unlock()
	done := make(chan struct{})
	go func() { defer close(done); f.s.ProcessUpload(context.Background(), id) }()
	defer func() { close(release); <-done }()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		j, _ := f.s.Store.Upload(id)
		if strings.Contains(j.Message, "waiting for an answer") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("prompt not saved: %q", j.Message)
		}
	}
}

func TestUploadQueueSpacesSubmissionsWithoutRepeatedUAProbe(t *testing.T) {
	f := newUploadFixture(t)
	f.s.Config.UploadSubmissionInterval = 100 * time.Millisecond
	f.mu.Lock()
	f.uniqueHashes, f.progress, f.state, f.files = true, 1, "stalledUP", true
	f.mu.Unlock()
	first, second := f.queue(t, "first"), f.queue(t, "second")
	f.s.ProcessUpload(context.Background(), first)
	f.s.ProcessUpload(context.Background(), second)
	j, _ := f.s.Store.Upload(second)
	f.mu.Lock()
	calls, probes := f.calls, f.browseCalls
	f.mu.Unlock()
	if calls != 1 || j.Status != "downloading" || !strings.Contains(j.Message, "queued") || probes != 0 {
		t.Fatalf("second job was not paced: calls=%d probes=%d job=%+v", calls, probes, j)
	}
	time.Sleep(110 * time.Millisecond)
	f.s.ProcessUpload(context.Background(), second)
	j, _ = f.s.Store.Upload(second)
	f.mu.Lock()
	calls = f.calls
	f.mu.Unlock()
	if calls != 2 || j.Status != "finished" {
		t.Fatalf("queued job did not run: calls=%d job=%+v", calls, j)
	}
}

func TestExplicitRateLimitQueuesSafeRetry(t *testing.T) {
	f := newUploadFixture(t)
	f.mu.Lock()
	f.uniqueHashes, f.progress, f.state, f.files = true, 1, "stalledUP", true
	f.rateLimitRemaining, f.retryAfter = 1, "120"
	f.mu.Unlock()
	first, second := f.queue(t, "first"), f.queue(t, "second")
	f.s.ProcessUpload(context.Background(), first)
	j, _ := f.s.Store.Upload(first)
	if j.Status != "downloading" || j.RateLimitRetries != 1 || j.NextAttemptAt == nil || time.Until(*j.NextAttemptAt) < 119*time.Second {
		t.Fatalf("429 did not defer retry: %+v", j)
	}
	f.s.ProcessUpload(context.Background(), second)
	other, _ := f.s.Store.Upload(second)
	f.mu.Lock()
	calls := f.calls
	f.mu.Unlock()
	if calls != 1 || other.Status != "downloading" {
		t.Fatalf("429 did not pause the queue: calls=%d job=%+v", calls, other)
	}
	// Advance the saved retry deadline to test that the original reservation is reused.
	past := time.Now().Add(-time.Second)
	j.NextAttemptAt = &past
	if err := f.s.Store.SaveUpload(j); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	f.s.ProcessUpload(context.Background(), first)
	j, _ = f.s.Store.Upload(first)
	f.mu.Lock()
	calls = f.calls
	f.mu.Unlock()
	if calls != 2 || j.Status != "finished" || j.NextAttemptAt != nil {
		t.Fatalf("safe retry did not complete: calls=%d job=%+v", calls, j)
	}
}

func TestUploadSettingsRedactionAndDestinationChange(t *testing.T) {
	f := newUploadFixture(t)
	v, _ := f.s.UploadSettings()
	if v.Token != "" || !v.HasToken {
		t.Fatal("secret returned")
	}
	v.URL = "https://other.example"
	if err := f.s.ConfigureUploads(context.Background(), v, true); err == nil {
		t.Fatal("secret forwarded to new destination")
	}
	id := f.queue(t, "search1")
	v, _ = f.s.Store.UploadSettings()
	v.Enabled = false
	_ = f.s.Store.SaveUploadSettings(v)
	f.s.ProcessUpload(context.Background(), id)
	j, _ := f.s.Store.Upload(id)
	if j.Status != "downloading" || !strings.Contains(j.Message, "disabled") {
		t.Fatal("disabled queue ran")
	}
}
