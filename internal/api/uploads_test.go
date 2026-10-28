package api

import (
	"encoding/json"
	"ravenseed/internal/models"
	"strings"
	"testing"
)

func TestUploadSettingsAndQueueAPI(t *testing.T) {
	f := newFixture(t, false)
	v := models.UploadSettings{Enabled: true, URL: "http://upload-assistant:5000", Token: "upload-secret", Trackers: []models.UploadTracker{{IndexerID: 3, Code: "AITHER"}}, Paths: []models.UploadPath{{InstanceID: 7, From: "/downloads", To: "/data"}}}
	code, body := f.do("PUT", "/api/v1/settings/uploads", v, "")
	if code != 200 || strings.Contains(string(body), "upload-secret") {
		t.Fatalf("settings response: %d %s", code, body)
	}
	job := f.start(req())
	f.wait(job.ID)
	rows, _ := f.store.Results(job.ID)
	code, body = f.do("POST", "/api/v1/searches/"+job.ID+"/results/"+rows[0].ID+"/add", models.AddTorrentRequest{InstanceID: 7, UploadAfterDownload: true}, "")
	if code != 200 || !strings.Contains(string(body), "uploadJobId") {
		t.Fatalf("queue add: %d %s", code, body)
	}
	add := <-f.adds
	if !strings.Contains(add["tags"], "ravenseed-") || add["paused"] != "false" {
		t.Fatal("tracking tag or download start missing")
	}
	jobs, total, _, _ := f.store.UploadQueue(20, 0, nil)
	if total != 1 || jobs[0].Tracker != "AITHER" {
		t.Fatal("queue not persisted")
	}
	code, body = f.do("GET", "/api/v1/uploads", nil, "")
	if code != 200 || strings.Contains(string(body), "upload-secret") || !strings.Contains(string(body), "downloading") {
		t.Fatal("unsafe queue response")
	}
	for query, want := range map[string]int{"?status=downloading": 1, "?status=adding,downloading,uploading": 1, "?status=failed,needs_review": 0, "?status=finished&status=cancelled": 0} {
		code, body = f.do("GET", "/api/v1/uploads"+query, nil, "")
		var page UploadJobsPage
		if code != 200 || json.Unmarshal(body, &page) != nil || page.Total != want || len(page.Items) != want || page.StatusCounts["downloading"] != 1 || page.StatusCounts["finished"] != 0 {
			t.Fatalf("%s: %d %s", query, code, body)
		}
	}
	if code, _ := f.do("GET", "/api/v1/uploads?status=downloading,bogus", nil, ""); code != 400 {
		t.Fatal("unknown upload status accepted")
	}
	if code, _ := f.do("POST", "/api/v1/uploads/"+jobs[0].ID+"/cancel", map[string]any{}, ""); code != 200 {
		t.Fatal("cancel failed")
	}
	path := "/api/v1/uploads/" + jobs[0].ID + "/retry"
	if code, _ := f.do("POST", path, map[string]any{"args": "--trackers OTHER"}, ""); code != 400 {
		t.Fatal("reserved retry flags accepted")
	}
	code, body = f.do("POST", path, map[string]any{"attempt": 0, "args": "--anon --modq", "tmdbId": 603}, "")
	if code != 200 || !strings.Contains(string(body), `"attempt":1`) || !strings.Contains(string(body), `"tmdbId":603`) {
		t.Fatalf("retry failed: %d %s", code, body)
	}
	if code, _ := f.do("POST", path, map[string]any{"attempt": 0, "args": "--anon"}, ""); code != 400 {
		t.Fatal("duplicate retry accepted")
	}
}
