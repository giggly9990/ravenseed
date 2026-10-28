package api

import (
	"encoding/json"
	"fmt"
	"net/url"
	"ravenseed/internal/models"
	"strings"
	"testing"
	"time"
)

func TestMonitorPauseResumeAPI(t *testing.T) {
	f := newFixture(t, false)
	m := models.Monitor{ID: "mon1", SourceIndexerID: 1, SourceTrackerHost: "a.example", TargetIndexerID: 2, IntervalHours: 24, Enabled: true, CreatedAt: time.Now().UTC()}
	if err := f.store.CreateMonitor(m); err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{false, true} {
		code, body := f.do("PUT", "/api/v1/monitors/mon1", map[string]bool{"enabled": enabled}, "")
		if code != 200 {
			t.Fatalf("state update failed: %d %s", code, body)
		}
		var updated models.Monitor
		if err := json.Unmarshal(body, &updated); err != nil || updated.Enabled != enabled {
			t.Fatalf("wrong state response: %+v %v", updated, err)
		}
		saved, err := f.store.Monitor(m.ID)
		if err != nil || saved.Enabled != enabled {
			t.Fatalf("state not persisted: %+v %v", saved, err)
		}
	}
	if code, _ := f.do("PUT", "/api/v1/monitors/mon1", map[string]any{}, ""); code != 400 {
		t.Fatalf("missing enabled accepted: %d", code)
	}
	if code, _ := f.do("PUT", "/api/v1/monitors/missing", map[string]bool{"enabled": false}, ""); code != 404 {
		t.Fatalf("missing monitor returned %d", code)
	}
	// instanceIds edits the scope; [] selects every instance.
	if code, body := f.do("PUT", "/api/v1/monitors/mon1", map[string]any{"instanceIds": []int{}}, ""); code != 400 || !strings.Contains(string(body), "qui connection changed") {
		t.Fatalf("all-instances edit for another connection: %d %s", code, body)
	}
	if code, _ := f.do("PUT", "/api/v1/monitors/missing", map[string]any{"instanceIds": []int{7}}, ""); code != 404 {
		t.Fatalf("missing monitor returned %d", code)
	}
	// mon1 was saved for another qui connection; nothing changes.
	code, body := f.do("PUT", "/api/v1/monitors/mon1", map[string]any{"instanceIds": []int{7}, "enabled": false}, "")
	if code != 400 || !strings.Contains(string(body), "qui connection changed") {
		t.Fatalf("instance edit: %d %s", code, body)
	}
	if saved, err := f.store.Monitor(m.ID); err != nil || !saved.Enabled || saved.InstanceIDs != nil {
		t.Fatalf("rejected edit persisted: %+v %v", saved, err)
	}
}

func TestMonitorCandidateSearchSpansPages(t *testing.T) {
	f := newFixture(t, false)
	m := models.Monitor{ID: "mon1", SourceIndexerID: 1, SourceTrackerHost: "a.example", TargetIndexerID: 2, IntervalHours: 24, Enabled: true, CreatedAt: time.Now().UTC()}
	if err := f.store.CreateMonitor(m); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 60; i++ {
		name := fmt.Sprintf("Filler.Show.S%02d.1080p.WEB-DL.H.264-GRP", i+1)
		if i == 55 {
			name = "Scrubs.S06.DSNP.WEB-DL.AAC2.0.H.264-playWEB"
		}
		c := models.MonitorCandidate{ID: fmt.Sprintf("c%d", i), MonitorID: "mon1", InstanceID: 7, Hash: fmt.Sprintf("%040d", i), Name: name, Status: "queued", NextCheckAt: time.Now().Add(time.Duration(i) * time.Minute)}
		c.Parsed.Group = "GRP"
		if i == 55 {
			c.Parsed.Group = "PLAYWEB"
		}
		if err := f.store.InsertMonitorCandidate(c); err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []string{"scrubs", "Scrubs S06", "scrubs.s06", "playweb"} {
		code, body := f.do("GET", "/api/v1/monitors/mon1/candidates?limit=50&offset=0&q="+url.QueryEscape(q), nil, "")
		var page struct {
			Items []models.MonitorCandidate `json:"items"`
			Total int                       `json:"total"`
		}
		if code != 200 || json.Unmarshal(body, &page) != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != "c55" {
			t.Fatalf("%q: %d %s", q, code, body)
		}
	}
	code, body := f.do("GET", "/api/v1/monitors/mon1/candidates?limit=50&offset=0&q=", nil, "")
	if code != 200 || !strings.Contains(string(body), `"total":60`) {
		t.Fatalf("empty search filtered rows: %d", code)
	}
}

func TestMonitorCandidatesQueuedFilterAndCheckedSort(t *testing.T) {
	f := newFixture(t, false)
	if err := f.store.CreateMonitor(models.Monitor{ID: "mon1", SourceIndexerID: 1, SourceTrackerHost: "a.example", TargetIndexerID: 2, IntervalHours: 24, Enabled: true, CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	at := func(h int) *time.Time { v := now.Add(time.Duration(-h) * time.Hour); return &v }
	for _, c := range []models.MonitorCandidate{
		{ID: "old", Status: "candidate", LastCheckedAt: at(30)},
		{ID: "new", Status: "present", LastCheckedAt: at(1)},
		{ID: "waiting", Status: "queued"},
		{ID: "running", Status: "checking", LastCheckedAt: at(10)},
	} {
		c.MonitorID, c.InstanceID, c.Hash, c.Name, c.NextCheckAt = "mon1", 7, fmt.Sprintf("%040s", c.ID), c.ID, now
		if err := f.store.InsertMonitorCandidate(c); err != nil {
			t.Fatal(err)
		}
		if err := f.store.SaveMonitorCandidate(c); err != nil {
			t.Fatal(err)
		}
	}
	ids := func(query string) []string {
		t.Helper()
		code, body := f.do("GET", "/api/v1/monitors/mon1/candidates?limit=50&offset=0&"+query, nil, "")
		var page struct {
			Items []models.MonitorCandidate `json:"items"`
		}
		if code != 200 || json.Unmarshal(body, &page) != nil {
			t.Fatalf("%s: %d %s", query, code, body)
		}
		out := []string{}
		for _, c := range page.Items {
			out = append(out, c.ID)
		}
		return out
	}
	if got := strings.Join(ids("classification=queued"), ","); got != "running,waiting" && got != "waiting,running" {
		t.Fatalf("queued filter: %s", got)
	}
	if got := strings.Join(ids("sort=checked_desc"), ","); got != "new,running,old,waiting" {
		t.Fatalf("newest checked first: %s", got)
	}
	if got := strings.Join(ids("sort=checked_asc"), ","); got != "old,running,new,waiting" {
		t.Fatalf("oldest checked first: %s", got)
	}
	if code, _ := f.do("GET", "/api/v1/monitors/mon1/candidates?sort=size", nil, ""); code != 400 {
		t.Fatalf("unknown sort accepted: %d", code)
	}
}

func TestMonitorCandidateCountsMatchListTotals(t *testing.T) {
	f := newFixture(t, false)
	now := time.Now().UTC()
	for _, id := range []string{"mon1", "mon2", "empty"} {
		if err := f.store.CreateMonitor(models.Monitor{ID: id, SourceIndexerID: 1, SourceTrackerHost: id + ".example", TargetIndexerID: 2, IntervalHours: 24, Enabled: true, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	policy := func(class models.CandidateClassification) *models.PolicyEvaluation {
		return &models.PolicyEvaluation{Classification: class}
	}
	for i, c := range []models.MonitorCandidate{
		{MonitorID: "mon1", Status: "candidate", Policy: policy(models.PolicyTrump)},
		{MonitorID: "mon1", Status: "candidate", Policy: policy(models.PolicyMissingSlot)},
		{MonitorID: "mon1", Status: "candidate", Policy: policy(models.PolicyMissingSlot)},
		{MonitorID: "mon1", Status: "uncertain", Policy: policy(models.PolicyPossibleTrump)},
		{MonitorID: "mon1", Status: "present", Policy: policy(models.PolicyEquivalent)},
		{MonitorID: "mon1", Status: "occupied", Policy: policy(models.PolicyNotEligible)},
		// A stale policy still counts under its classification.
		{MonitorID: "mon1", Status: "error", Policy: policy(models.PolicyCoexistence)},
		{MonitorID: "mon1", Status: "queued", Policy: policy(models.PolicyTrump)},
		{MonitorID: "mon1", Status: "checking"},
		{MonitorID: "mon1", Status: "gone"},
		{MonitorID: "mon2", Status: "error"},
		{MonitorID: "mon2", Status: "candidate", Policy: policy(models.PolicyCoexistence)},
		// No filter lists unevaluated candidates apart from all.
		{MonitorID: "mon2", Status: "uncertain", Policy: policy(models.PolicyUnevaluated)},
	} {
		c.ID = fmt.Sprintf("c%d", i)
		c.InstanceID, c.Hash, c.Name, c.NextCheckAt = 7, fmt.Sprintf("%040d", i), c.ID, now
		if err := f.store.InsertMonitorCandidate(c); err != nil {
			t.Fatal(err)
		}
		if err := f.store.SaveMonitorCandidate(c); err != nil {
			t.Fatal(err)
		}
	}
	code, body := f.do("GET", "/api/v1/monitor-candidate-counts", nil, "")
	var counts map[string]map[string]int
	if code != 200 || json.Unmarshal(body, &counts) != nil || len(counts) != 3 {
		t.Fatalf("counts: %d %s", code, body)
	}
	if counts["mon1"]["all"] != 10 || counts["mon1"]["trump"] != 2 || counts["mon1"]["queued"] != 2 || counts["mon2"]["errors"] != 1 {
		t.Fatalf("unexpected counts: %v", counts)
	}
	for monitor, row := range counts {
		if len(row) != 10 {
			t.Fatalf("%s: missing classes: %v", monitor, row)
		}
		for class, n := range row {
			code, body := f.do("GET", "/api/v1/monitors/"+monitor+"/candidates?limit=1&offset=0&classification="+class, nil, "")
			var page struct {
				Total int `json:"total"`
			}
			if code != 200 || json.Unmarshal(body, &page) != nil || page.Total != n {
				t.Fatalf("%s %s: count %d, list %d %s", monitor, class, n, code, body)
			}
		}
	}
}

func TestUpgradesListAndDismiss(t *testing.T) {
	f := newFixture(t, false)
	if err := f.store.CreateMonitor(models.Monitor{ID: "mon1", SourceIndexerID: 1, SourceTrackerHost: "a.example", TargetIndexerID: 2, IntervalHours: 24, Enabled: true, CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	offer := func(title string) *models.PolicyTargetResult {
		return &models.PolicyTargetResult{Title: title, Size: 5000, Relation: "same_slot"}
	}
	for _, c := range []models.MonitorCandidate{
		{ID: "repack", Name: "Odd.Thomas.2013.1080p.BluRay.x264-GRP", Upgrade: "upgrade", UpgradeOffer: offer("Odd.Thomas.2013.REPACK.1080p.BluRay.x264-GRP")},
		{ID: "other", Name: "Scrubs.S06.1080p.WEB-DL-AAA", Upgrade: "possible", UpgradeOffer: offer("Scrubs.S06.1080p.WEB-DL-BBB")},
		{ID: "none", Name: "Nothing.2020.1080p.WEB-DL-CCC"},
	} {
		c.MonitorID, c.InstanceID, c.Hash, c.Status, c.NextCheckAt = "mon1", 7, fmt.Sprintf("%040s", c.ID), "not_eligible", time.Now()
		if err := f.store.InsertMonitorCandidate(c); err != nil {
			t.Fatal(err)
		}
		if err := f.store.SaveMonitorCandidate(c); err != nil {
			t.Fatal(err)
		}
	}
	list := func(query string) []string {
		t.Helper()
		code, body := f.do("GET", "/api/v1/upgrades?"+query, nil, "")
		var page struct {
			Items []models.MonitorCandidate `json:"items"`
		}
		if code != 200 || json.Unmarshal(body, &page) != nil {
			t.Fatalf("%s: %d %s", query, code, body)
		}
		ids := []string{}
		for _, c := range page.Items {
			ids = append(ids, c.ID)
		}
		return ids
	}
	if got := strings.Join(list(""), ","); got != "repack,other" {
		t.Fatalf("upgrades: %s", got)
	}
	if got := strings.Join(list("level=possible&monitorId=mon1"), ","); got != "other" {
		t.Fatalf("possible: %s", got)
	}
	if got := strings.Join(list("q=odd%20thomas"), ","); got != "repack" {
		t.Fatalf("search: %s", got)
	}
	if code, _ := f.do("POST", "/api/v1/monitor-candidates/repack/upgrade/dismiss", map[string]bool{"dismissed": true}, ""); code != 200 {
		t.Fatalf("dismiss: %d", code)
	}
	if got := strings.Join(list(""), ","); got != "other" {
		t.Fatalf("dismissed upgrade still listed: %s", got)
	}
	if got := strings.Join(list("dismissed=true"), ","); got != "repack" {
		t.Fatalf("dismissed list: %s", got)
	}
	if code, _ := f.do("POST", "/api/v1/monitor-candidates/repack/upgrade/dismiss", map[string]bool{"dismissed": false}, ""); code != 200 || strings.Join(list(""), ",") != "repack,other" {
		t.Fatalf("restore failed: %d", code)
	}
	if code, _ := f.do("POST", "/api/v1/monitor-candidates/none/upgrade/dismiss", map[string]bool{"dismissed": true}, ""); code != 400 {
		t.Fatalf("dismissing a torrent without an upgrade: %d", code)
	}
	if code, _ := f.do("GET", "/api/v1/upgrades?level=best", nil, ""); code != 400 {
		t.Fatalf("unknown level accepted: %d", code)
	}
}
