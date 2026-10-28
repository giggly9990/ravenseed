package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"ravenseed/internal/models"
	"strings"
	"time"
)

func (s *Store) UploadSettings() (models.UploadSettings, error) {
	v := models.UploadSettings{Trackers: []models.UploadTracker{}, Paths: []models.UploadPath{}}
	var b []byte
	err := s.DB.QueryRow("SELECT payload FROM upload_settings WHERE id=1").Scan(&b)
	if err == sql.ErrNoRows {
		return v, nil
	}
	if err != nil {
		return v, err
	}
	return v, json.Unmarshal(b, &v)
}

func (s *Store) SaveUploadSettings(v models.UploadSettings) error {
	b, err := encode(v)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec("INSERT INTO upload_settings(id,payload) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload", b)
	return err
}

func (s *Store) CreateUpload(v models.UploadJob) error {
	b, err := encode(v)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec("INSERT INTO upload_jobs(id,search_id,result_id,instance_id,target_id,status,created_at,payload) VALUES(?,?,?,?,?,?,?,?)", v.ID, v.SearchID, v.ResultID, v.InstanceID, v.TargetIndexerID, v.Status, stamp(v.CreatedAt), b)
	return err
}

func (s *Store) SaveUpload(v models.UploadJob) error {
	v.UpdatedAt = time.Now().UTC()
	b, err := encode(v)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec("UPDATE upload_jobs SET status=?,payload=? WHERE id=?", v.Status, b, v.ID)
	return err
}

// ErrUploadChanged reports a waiting job that was cancelled, retried or
// claimed meanwhile; the stale write is dropped.
var ErrUploadChanged = errors.New("upload job changed meanwhile")

func (s *Store) SaveWaitingUpload(v models.UploadJob) error {
	v.UpdatedAt = time.Now().UTC()
	b, err := encode(v)
	if err != nil {
		return err
	}
	r, err := s.DB.Exec("UPDATE upload_jobs SET status=?,payload=? WHERE id=? AND status='downloading' AND COALESCE(json_extract(payload,'$.attempt'),0)=?", v.Status, b, v.ID, v.Attempt)
	if err != nil {
		return err
	}
	if n, err := r.RowsAffected(); err != nil || n == 1 {
		return err
	}
	return ErrUploadChanged
}

func (s *Store) Upload(id string) (models.UploadJob, error) {
	var v models.UploadJob
	var b []byte
	err := s.DB.QueryRow("SELECT payload FROM upload_jobs WHERE id=?", id).Scan(&b)
	if err != nil {
		return v, err
	}
	return v, json.Unmarshal(b, &v)
}

// UploadQueue returns one page of upload jobs, newest first, their total and
// the number of jobs in each status, counted over all jobs in one grouped
// query. Statuses, when given, limit the page and its total to those statuses.
func (s *Store) UploadQueue(limit, offset int, statuses []string) ([]models.UploadJob, int, map[string]int, error) {
	counts := map[string]int{}
	for _, status := range models.UploadStatuses {
		counts[status] = 0
	}
	rows, err := s.DB.Query("SELECT status, count(*) FROM upload_jobs GROUP BY status")
	if err != nil {
		return nil, 0, nil, err
	}
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			rows.Close()
			return nil, 0, nil, err
		}
		counts[status] = n
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, 0, nil, err
	}
	total := 0
	query, args := "SELECT payload FROM upload_jobs", []any{}
	if len(statuses) == 0 {
		for _, n := range counts {
			total += n
		}
	} else {
		seen := map[string]bool{}
		for _, status := range statuses {
			if !seen[status] {
				seen[status] = true
				total += counts[status]
				args = append(args, status)
			}
		}
		query += " WHERE status IN (?" + strings.Repeat(",?", len(args)-1) + ")"
	}
	items, err := read[models.UploadJob](s, query+" ORDER BY created_at DESC LIMIT ? OFFSET ?", append(args, limit, offset)...)
	if err != nil {
		return nil, 0, nil, err
	}
	return items, total, counts, nil
}

func (s *Store) PendingUploads() ([]models.UploadJob, error) {
	return read[models.UploadJob](s, "SELECT payload FROM upload_jobs WHERE status=? ORDER BY created_at", "downloading")
}

// UploadDispatchAfter keeps the submission cadence and a 429 cooldown across restarts.
func (s *Store) UploadDispatchAfter(assistantURL string, interval time.Duration) (time.Time, error) {
	// Only recent submissions and waiting retries can delay the next run.
	rows, err := s.DB.Query(`SELECT json_extract(payload,'$.submittedAt'), CASE WHEN status='downloading' THEN json_extract(payload,'$.nextAttemptAt') END FROM upload_jobs
		WHERE json_extract(payload,'$.assistantUrl')=? AND (status='downloading' OR julianday(json_extract(payload,'$.submittedAt'))>=julianday(?))`, assistantURL, stamp(time.Now().Add(-interval-time.Minute)))
	if err != nil {
		return time.Time{}, err
	}
	defer rows.Close()
	var after time.Time
	for rows.Next() {
		var submitted, retry sql.NullString
		if err := rows.Scan(&submitted, &retry); err != nil {
			return time.Time{}, err
		}
		for _, item := range []struct {
			value sql.NullString
			add   time.Duration
		}{{submitted, interval}, {retry, 0}} {
			if !item.value.Valid || item.value.String == "" {
				continue
			}
			t, err := time.Parse(time.RFC3339Nano, item.value.String)
			if err != nil {
				return time.Time{}, err
			}
			if t = t.Add(item.add); t.After(after) {
				after = t
			}
		}
	}
	return after, rows.Err()
}

func (s *Store) ClaimUpload(v models.UploadJob) (bool, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	r, err := tx.Exec("INSERT OR IGNORE INTO upload_claims(connection,instance_id,hash,tracker,job_id) VALUES(?,?,?,?,?)", v.QuiURL, v.InstanceID, v.Hash, v.Tracker, v.ID)
	if err != nil {
		return false, err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return false, err
	}
	if n != 1 {
		var owner string
		if err := tx.QueryRow("SELECT job_id FROM upload_claims WHERE connection=? AND instance_id=? AND hash=? AND tracker=?", v.QuiURL, v.InstanceID, v.Hash, v.Tracker).Scan(&owner); err != nil {
			return false, err
		}
		if (v.Attempt == 0 && v.RateLimitRetries == 0) || owner != v.ID {
			return false, nil
		}
	}
	v.UpdatedAt = time.Now().UTC()
	b, err := encode(v)
	if err != nil {
		return false, err
	}
	r, err = tx.Exec("UPDATE upload_jobs SET status=?,payload=? WHERE id=? AND status='downloading' AND COALESCE(json_extract(payload,'$.attempt'),0)=?", v.Status, b, v.ID, v.Attempt)
	if err != nil {
		return false, err
	}
	n, err = r.RowsAffected()
	if err != nil || n != 1 {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) RecoverUploads() error {
	for _, status := range []string{"adding", "uploading"} {
		jobs, err := read[models.UploadJob](s, "SELECT payload FROM upload_jobs WHERE status=?", status)
		if err != nil {
			return err
		}
		for _, j := range jobs {
			j.Status, j.Message = "needs_review", "Service restarted during a submission. Check qui and Upload-Assistant before taking further action."
			if err := s.SaveUpload(j); err != nil {
				return err
			}
		}
	}
	return nil
}
