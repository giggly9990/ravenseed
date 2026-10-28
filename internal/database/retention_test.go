package database

import (
	"context"
	"encoding/json"
	"path/filepath"
	"ravenseed/internal/models"
	"strings"
	"testing"
	"time"
)

func TestRetentionKeepsActiveWorkAndDownloadURLs(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "retention.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	now := time.Now().UTC()
	for _, id := range []string{"old", "protected", "running", "recent"} {
		j := models.Job{ID: id, Status: "completed", StartedAt: now.Add(-48 * time.Hour)}
		if id == "recent" {
			j.StartedAt = now
		}
		if id == "running" {
			j.Status = "running"
		}
		if err := s.SaveJob(j, id); err != nil {
			t.Fatal(err)
		}
		r := models.Result{ID: "r", Size: 9007199254740993, DownloadURL: "https://tracker/file?passkey=keep", Raw: json.RawMessage(`{"secret":"remove"}`)}
		if err := s.SaveAnalysis(j, []models.Result{r}, []models.Family{{ID: "f", Results: []models.Result{r}}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.CreateUpload(models.UploadJob{ID: "u", SearchID: "protected", Status: "needs_review", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.Prune(context.Background(), now, 0, time.Hour); err != nil {
		t.Fatal(err)
	}
	results, err := s.Results("old")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || len(results[0].Raw) != 0 || results[0].Size != 9007199254740993 || results[0].DownloadURL != "https://tracker/file?passkey=keep" {
		t.Fatal("raw pruning lost download or numeric data")
	}
	families, _ := s.Families("old")
	if len(families[0].Results[0].Raw) != 0 {
		t.Fatal("family retained duplicate raw response")
	}
	results, _ = s.Results("running")
	if len(results[0].Raw) == 0 {
		t.Fatal("running search pruned")
	}
	if err := s.Prune(context.Background(), now, time.Hour, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Job("old"); err == nil {
		t.Fatal("expired history retained")
	}
	for _, id := range []string{"protected", "running", "recent"} {
		if _, err := s.Job(id); err != nil {
			t.Fatal("protected history removed", id)
		}
	}
	rows, _ := s.Results("old")
	if len(rows) != 0 {
		t.Fatal("results did not cascade")
	}
}

func TestRecoveryAllUploadStates(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "recover.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	for _, status := range []string{"adding", "downloading", "uploading", "needs_review", "finished", "failed", "cancelled"} {
		if err := s.CreateUpload(models.UploadJob{ID: status, SearchID: status, Status: status, CreatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.RecoverUploads(); err != nil {
		t.Fatal(err)
	}
	if err := s.RecoverUploads(); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"adding", "downloading", "uploading", "needs_review", "finished", "failed", "cancelled"} {
		j, err := s.Upload(status)
		if err != nil {
			t.Fatal(err)
		}
		want := status
		if status == "adding" || status == "uploading" {
			want = "needs_review"
		}
		if j.Status != want {
			t.Fatalf("%s recovered to %s", status, j.Status)
		}
	}
}

func TestPruneRawPreservesTrackerFlagPresence(t *testing.T) {
	for _, test := range []struct {
		name, input, want string
	}{
		{"legacy missing flags", `{"downloadFactor":0,"uploadFactor":0,"raw":{}}`, `{}`},
		{"explicit freeleech", `{"downloadFactor":0,"raw":{"download_volume_factor":0,"upload_volume_factor":2}}`, `{"downloadFactor":0,"uploadFactor":2}`},
		{"already pruned", `{"downloadFactor":0}`, `{"downloadFactor":0}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			var value any
			decoder := json.NewDecoder(strings.NewReader(test.input))
			decoder.UseNumber()
			if err := decoder.Decode(&value); err != nil {
				t.Fatal(err)
			}
			stripRaw(value)
			body, err := json.Marshal(value)
			if err != nil || string(body) != test.want {
				t.Fatalf("got %s, want %s: %v", body, test.want, err)
			}
		})
	}
}

func TestRetentionReleasesFailedUploadsForRequeue(t *testing.T) {
	s := openTest(t)
	now := time.Now().UTC()
	// "late" was created long ago but failed just now; it is kept until the
	// retention period has passed since it ended.
	for id, status := range map[string]string{"failed": "failed", "cancelled": "cancelled", "done": "finished", "late": "failed"} {
		j := models.UploadJob{ID: id, SearchID: "m", ResultID: id, Status: status, CreatedAt: now.Add(-48 * time.Hour), UpdatedAt: now.Add(-47 * time.Hour)}
		if err := s.CreateUpload(j); err != nil {
			t.Fatal(err)
		}
		if id == "late" {
			if err := s.SaveUpload(j); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := s.DB.Exec("INSERT INTO upload_claims(connection,instance_id,hash,tracker,job_id) VALUES('q',7,?,'AITHER',?)", id, id); err != nil {
			t.Fatal(err)
		}
		addCandidate(t, s, models.MonitorCandidate{ID: id, Status: "candidate", UploadJobID: id, NextCheckAt: now})
	}
	if err := s.Prune(context.Background(), now, time.Hour, 0); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"failed", "cancelled"} {
		if v, _ := s.MonitorCandidate(id); v.UploadJobID != "" || v.Status != "candidate" {
			t.Fatalf("%s upload still linked: %+v", id, v)
		}
		if _, err := s.Upload(id); err == nil {
			t.Fatalf("%s upload retained", id)
		}
	}
	for _, id := range []string{"done", "late"} {
		if v, _ := s.MonitorCandidate(id); v.UploadJobID != id {
			t.Fatalf("%s upload unlinked: %+v", id, v)
		}
	}
	if j, err := s.Upload("late"); err != nil || j.Status != "failed" {
		t.Fatalf("job that just failed was pruned: %v", err)
	}
	var claims []string
	rows, _ := s.DB.Query("SELECT job_id FROM upload_claims ORDER BY job_id")
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		claims = append(claims, id)
	}
	rows.Close()
	if strings.Join(claims, ",") != "done,late" {
		t.Fatalf("claims: %v", claims)
	}
	if err := s.SetCandidateUploadJob("failed", "upload_new"); err != nil {
		t.Fatalf("candidate cannot be queued again: %v", err)
	}
	if err := s.Prune(context.Background(), now.Add(2*time.Hour), time.Hour, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Upload("late"); err == nil {
		t.Fatal("failed job retained past its retention period")
	}
}
