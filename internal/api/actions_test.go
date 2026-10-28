package api

import (
	"encoding/json"
	"fmt"
	"ravenseed/internal/models"
	"strings"
	"testing"
)

func TestPresetsPersistUpdateAndDelete(t *testing.T) {
	f := newFixture(t, false)
	input := models.IndexerPreset{Name: " Movie trackers ", IndexerIDs: []int{3, 1, 1}, TargetIndexerID: 3}
	code, body := f.do("PUT", "/api/v1/presets", input, "")
	if code != 200 {
		t.Fatalf("save: %d %s", code, body)
	}
	var saved models.IndexerPreset
	if err := json.Unmarshal(body, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.ID == "" || saved.Name != "Movie trackers" || len(saved.IndexerIDs) != 2 {
		t.Fatalf("bad preset: %+v", saved)
	}
	stored, err := f.store.Presets()
	if err != nil || len(stored) != 1 || stored[0].ID != saved.ID {
		t.Fatal("preset was not persisted")
	}
	input.Name = "movie TRACKERS"
	if code, _ := f.do("PUT", "/api/v1/presets", input, ""); code != 400 {
		t.Fatal("duplicate preset name accepted")
	}
	saved.IndexerIDs = []int{1, 2}
	saved.TargetIndexerID = 2
	if code, _ := f.do("PUT", "/api/v1/presets", saved, ""); code != 200 {
		t.Fatal("update failed")
	}
	if code, _ := f.do("DELETE", "/api/v1/presets/"+saved.ID, nil, ""); code != 200 {
		t.Fatal("delete failed")
	}
	stored, _ = f.store.Presets()
	if len(stored) != 0 {
		t.Fatal("preset was not deleted")
	}
	for _, invalid := range []models.IndexerPreset{{Name: "Empty"}, {Name: "Wrong target", IndexerIDs: []int{1}, TargetIndexerID: 3}, {Name: "Bad ID", IndexerIDs: []int{-1}}, {Name: "bad\nname", IndexerIDs: []int{1}}} {
		if code, _ := f.do("PUT", "/api/v1/presets", invalid, ""); code != 400 {
			t.Fatalf("invalid preset accepted: %+v", invalid)
		}
	}
}

func TestResolutionUsesSavedResultsAndPreservesFlags(t *testing.T) {
	f := newFixture(t, false)
	j := f.start(req())
	f.wait(j.ID)
	before := f.calls.Load()
	for resolution, expected := range map[string]int{"2160p": 1, "1080p": 0, "": 1} {
		code, body := f.do("GET", "/api/v1/searches/"+j.ID+"/families?resolution="+resolution, nil, "")
		var page Page[models.Family]
		if code != 200 || json.Unmarshal(body, &page) != nil || page.Total != expected {
			t.Fatalf("filter: %d %s", code, body)
		}
	}
	if f.calls.Load() != before {
		t.Fatal("resolution filter triggered an upstream search")
	}
	rows, _ := f.store.Results(j.ID)
	if len(rows) != 2 {
		t.Fatal("filter discarded stored results")
	}
	for _, row := range rows {
		if row.DownloadFactor == nil || *row.DownloadFactor != float64(row.IndexerID-1) || row.UploadFactor == nil || *row.UploadFactor != 2 {
			t.Fatal("tracker factors lost")
		}
	}
}

func TestAddSavedTorrentUsesQuiAndValidatesSelection(t *testing.T) {
	f := newFixture(t, false)
	j := f.start(req())
	f.wait(j.ID)
	rows, _ := f.store.Results(j.ID)
	path := fmt.Sprintf("/api/v1/searches/%s/results/%s/add", j.ID, rows[0].ID)
	input := models.AddTorrentRequest{InstanceID: 7, Category: "Movies", SavePath: "/downloads/movies", Tags: "ravenseed,manual"}
	code, body := f.do("POST", path, input, "")
	if code != 200 {
		t.Fatalf("add: %d %s", code, body)
	}
	values := <-f.adds
	if values["urls"] != rows[0].DownloadURL || values["indexer_id"] != fmt.Sprint(rows[0].IndexerID) || values["paused"] != "true" || values["category"] != input.Category || values["savepath"] != input.SavePath || values["tags"] != input.Tags || values["skip_checking"] != "" {
		t.Fatalf("incorrect add options: %+v", values)
	}
	paused := false
	input.Paused = &paused
	if code, _ := f.do("POST", path, input, ""); code != 200 {
		t.Fatal("unpaused add failed")
	}
	if (<-f.adds)["paused"] != "false" {
		t.Fatal("explicit unpaused option lost")
	}
	input.InstanceID = 8
	if code, _ := f.do("POST", path, input, ""); code != 400 {
		t.Fatal("offline instance accepted")
	}
	input.InstanceID = 7
	if code, _ := f.do("POST", "/api/v1/searches/"+j.ID+"/results/missing/add", input, ""); code != 404 {
		t.Fatal("unknown result accepted")
	}
	if code, _ := f.do("POST", path, map[string]any{"instanceId": 7, "downloadUrl": "https://untrusted.example"}, ""); code != 400 {
		t.Fatal("caller-supplied download URL accepted")
	}
	upload := input
	upload.UploadAfterDownload = true
	if code, body := f.do("POST", path, upload, ""); code != 400 || !strings.Contains(string(body), "enable Upload-Assistant") {
		t.Fatalf("upload without Upload-Assistant: %d %s", code, body)
	}
	if len(f.adds) != 0 {
		t.Fatal("invalid request reached qui")
	}
	f.addMode.Store(1)
	code, body = f.do("POST", path, input, "")
	if code != 502 || strings.Contains(string(body), "secret-qui-key") || strings.Contains(string(body), "private-download-token") || !strings.Contains(string(body), "check qui before retrying") {
		t.Fatalf("unsafe error response: %d %s", code, body)
	}
}

func TestResultsKeepDownloadLinksServerSide(t *testing.T) {
	f := newFixture(t, false)
	j := f.start(req())
	f.wait(j.ID)
	rows, _ := f.store.Results(j.ID)
	if len(rows) == 0 || rows[0].DownloadURL == "" {
		t.Fatal("download link not stored")
	}
	_, list := f.do("GET", "/api/v1/searches/"+j.ID+"/families", nil, "")
	var families struct {
		Items []models.Family `json:"items"`
	}
	if err := json.Unmarshal(list, &families); err != nil || len(families.Items) == 0 {
		t.Fatalf("families: %s", list)
	}
	for _, path := range []string{"/results", "/families", "/candidates", "/families/" + families.Items[0].ID} {
		code, body := f.do("GET", "/api/v1/searches/"+j.ID+path, nil, "")
		if code != 200 {
			t.Fatalf("%s: %d %s", path, code, body)
		}
		for _, leak := range []string{"private-download-token", "tracker.example/download", "download_url", "downloadUrl"} {
			if strings.Contains(string(body), leak) {
				t.Fatalf("%s leaks %q: %s", path, leak, body)
			}
		}
		if path != "/candidates" && !strings.Contains(string(body), `"hasDownloadUrl":true`) {
			t.Fatalf("%s: hasDownloadUrl missing: %s", path, body)
		}
	}
}

func TestCategoryPicker(t *testing.T) {
	f := newFixture(t, false)
	code, body := f.do("GET", "/api/v1/qui/instances/7/categories", nil, "")
	var names []string
	if code != 200 || json.Unmarshal(body, &names) != nil || len(names) != 2 || names[0] != "Movies" || names[1] != "TV/Anime" {
		t.Fatalf("categories: %d %s", code, body)
	}
	for _, id := range []string{"0", "-1", "bad"} {
		if code, _ := f.do("GET", "/api/v1/qui/instances/"+id+"/categories", nil, ""); code != 400 {
			t.Fatal("invalid instance accepted")
		}
	}
	code, body = f.do("GET", "/api/v1/qui/instances/8/categories", nil, "")
	if code != 502 || strings.Contains(string(body), "secret-qui-key") {
		t.Fatal("unsafe upstream failure")
	}
}

func TestNewActionsRequireAuthentication(t *testing.T) {
	f := newFixture(t, true)
	for _, path := range []string{"/api/v1/presets", "/api/v1/qui/instances", "/api/v1/qui/instances/7/categories", "/api/v1/uploads", "/api/v1/settings/uploads"} {
		if code, _ := f.do("GET", path, nil, ""); code != 401 {
			t.Fatal("unauthenticated read accepted")
		}
	}
	if code, _ := f.do("POST", "/api/v1/searches/id/results/id/add", models.AddTorrentRequest{InstanceID: 7}, ""); code != 401 {
		t.Fatal("unauthenticated add accepted")
	}
}
