package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"ravenseed/internal/models"
	"time"
)

func (s *Store) Monitors() ([]models.Monitor, error) {
	return read[models.Monitor](s, "SELECT payload FROM monitors ORDER BY rowid DESC")
}

func (s *Store) Monitor(id string) (models.Monitor, error) {
	var v models.Monitor
	var b []byte
	err := s.DB.QueryRow("SELECT payload FROM monitors WHERE id=?", id).Scan(&b)
	if err != nil {
		return v, err
	}
	return v, json.Unmarshal(b, &v)
}

func (s *Store) CreateMonitor(v models.Monitor) error {
	b, err := encode(v)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec("INSERT INTO monitors(id,source_indexer_id,source_tracker_host,target_indexer_id,enabled,payload) VALUES(?,?,?,?,?,?)", v.ID, v.SourceIndexerID, v.SourceTrackerHost, v.TargetIndexerID, v.Enabled, b)
	return err
}

// updateMonitor rewrites a monitor's payload inside one transaction.
func (s *Store) updateMonitor(id string, change func(*models.Monitor)) (models.Monitor, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return models.Monitor{}, err
	}
	defer tx.Rollback()
	var body []byte
	if err = tx.QueryRow("SELECT payload FROM monitors WHERE id=?", id).Scan(&body); err != nil {
		return models.Monitor{}, err
	}
	var v models.Monitor
	if err = json.Unmarshal(body, &v); err != nil {
		return v, err
	}
	change(&v)
	b, err := encode(v)
	if err != nil {
		return v, err
	}
	if _, err = tx.Exec("UPDATE monitors SET enabled=?,payload=? WHERE id=?", v.Enabled, b, id); err != nil {
		return v, err
	}
	return v, tx.Commit()
}

func (s *Store) SaveMonitorScan(v models.Monitor) error {
	_, err := s.updateMonitor(v.ID, func(current *models.Monitor) { current.LastScanAt, current.LastError = v.LastScanAt, v.LastError })
	return err
}

func (s *Store) SetMonitorEnabled(id string, enabled bool) (models.Monitor, error) {
	return s.updateMonitor(id, func(v *models.Monitor) { v.Enabled = enabled })
}

// SetMonitorInstances replaces the qui instances a monitor scans, none for
// every instance, and, when enabled is set, pauses or resumes it. The last
// scan error, which may concern the old selection, is cleared.
func (s *Store) SetMonitorInstances(id string, instances []int, enabled *bool) (models.Monitor, error) {
	return s.updateMonitor(id, func(v *models.Monitor) {
		v.InstanceIDs, v.LastError = instances, ""
		if enabled != nil {
			v.Enabled = *enabled
		}
	})
}

func (s *Store) DeleteMonitor(id string) (bool, error) {
	r, err := s.DB.Exec("DELETE FROM monitors WHERE id=?", id)
	if err != nil {
		return false, err
	}
	n, err := r.RowsAffected()
	return n > 0, err
}

func (s *Store) InsertMonitorCandidate(v models.MonitorCandidate) error {
	return s.InsertMonitorCandidates([]models.MonitorCandidate{v})
}

// InsertMonitorCandidates adds new torrents in one transaction; known
// torrents, paused or deleted monitors and instances the monitor no longer
// scans are skipped.
func (s *Store) InsertMonitorCandidates(items []models.MonitorCandidate) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	insert, err := tx.Prepare(`INSERT OR IGNORE INTO monitor_candidates(id,monitor_id,instance_id,hash,status,next_check_at,payload)
		SELECT ?,?,?,?,?,?,? WHERE EXISTS (SELECT 1 FROM monitors WHERE id=? AND enabled=1
		AND (COALESCE(json_array_length(payload,'$.instanceIds'),0)=0 OR ? IN (SELECT value FROM json_each(payload,'$.instanceIds'))))`)
	if err != nil {
		return err
	}
	defer insert.Close()
	for _, v := range items {
		b, err := encode(v)
		if err != nil {
			return err
		}
		if _, err = insert.Exec(v.ID, v.MonitorID, v.InstanceID, v.Hash, v.Status, stamp(v.NextCheckAt), b, v.MonitorID, v.InstanceID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// MonitorCandidateHashes lists the torrents a monitor already tracks, as
// instance|hash keys.
func (s *Store) MonitorCandidateHashes(monitorID string) (map[string]bool, error) {
	rows, err := s.DB.Query("SELECT instance_id,hash FROM monitor_candidates WHERE monitor_id=?", monitorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var instance int
		var hash string
		if err = rows.Scan(&instance, &hash); err != nil {
			return nil, err
		}
		out[fmt.Sprintf("%d|%s", instance, hash)] = true
	}
	return out, rows.Err()
}

func (s *Store) MonitorCandidate(id string) (models.MonitorCandidate, error) {
	var v models.MonitorCandidate
	var b []byte
	err := s.DB.QueryRow("SELECT payload FROM monitor_candidates WHERE id=?", id).Scan(&b)
	if err != nil {
		return v, err
	}
	return v, json.Unmarshal(b, &v)
}

func (s *Store) MonitorCandidates(monitorID string) ([]models.MonitorCandidate, error) {
	return read[models.MonitorCandidate](s, "SELECT payload FROM monitor_candidates WHERE monitor_id=? ORDER BY CASE status WHEN 'candidate' THEN 0 WHEN 'checking' THEN 1 WHEN 'queued' THEN 2 ELSE 3 END, next_check_at", monitorID)
}

// CandidateCountClasses are the classification filters of the monitor
// candidates list, each counted by MonitorCandidateCounts.
var CandidateCountClasses = []string{"all", "missing_slot", "trump", "possible_trump", "coexistence", "equivalent", "not_eligible", "queued", "errors", "gone"}

// MonitorCandidateCounts counts every monitor's candidates per
// classification filter in one grouped query, with the list filter's rules:
// "errors" and "gone" by status, "queued" for queued or checking, the others
// by policy classification (a candidate can count in a status and a
// classification). Every monitor has exactly the classes in
// CandidateCountClasses.
func (s *Store) MonitorCandidateCounts() (map[string]map[string]int, error) {
	rows, err := s.DB.Query(`SELECT m.id, json_extract(c.payload,'$.status'), json_extract(c.payload,'$.policy.classification'), count(c.id)
		FROM monitors m LEFT JOIN monitor_candidates c ON c.monitor_id=m.id GROUP BY 1,2,3`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]map[string]int{}
	for rows.Next() {
		var monitor string
		var status, class sql.NullString
		var n int
		if err := rows.Scan(&monitor, &status, &class, &n); err != nil {
			return nil, err
		}
		counts := out[monitor]
		if counts == nil {
			counts = make(map[string]int, len(CandidateCountClasses))
			for _, c := range CandidateCountClasses {
				counts[c] = 0
			}
			out[monitor] = counts
		}
		if n == 0 {
			continue
		}
		counts["all"] += n
		switch status.String {
		case "error":
			counts["errors"] += n
		case "gone":
			counts["gone"] += n
		case "queued", "checking":
			counts["queued"] += n
		}
		// Classifications without a list filter, such as unevaluated, are
		// counted only in all.
		if _, ok := counts[class.String]; ok {
			counts[class.String] += n
		}
	}
	return out, rows.Err()
}

func (s *Store) DueMonitorCandidate(now time.Time) (models.MonitorCandidate, error) {
	var v models.MonitorCandidate
	var b []byte
	err := s.DB.QueryRow(`SELECT c.payload FROM monitor_candidates c JOIN monitors m ON m.id=c.monitor_id
        WHERE m.enabled=1 AND c.status!='checking' AND c.next_check_at<=?
        ORDER BY c.next_check_at LIMIT 1`, stamp(now)).Scan(&b)
	if err != nil {
		return v, err
	}
	return v, json.Unmarshal(b, &v)
}

func (s *Store) ClaimMonitorCandidate(id string) (bool, error) {
	r, err := s.DB.Exec(`UPDATE monitor_candidates SET status='checking',payload=json_set(payload,'$.status','checking')
		WHERE id=? AND status!='checking' AND EXISTS (SELECT 1 FROM monitors WHERE monitors.id=monitor_candidates.monitor_id AND monitors.enabled=1)`, id)
	if err != nil {
		return false, err
	}
	n, err := r.RowsAffected()
	return n == 1, err
}

func (s *Store) SaveMonitorCandidate(v models.MonitorCandidate) error {
	b, err := encode(v)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec("UPDATE monitor_candidates SET status=?,next_check_at=?,payload=? WHERE id=?", v.Status, stamp(v.NextCheckAt), b, v.ID)
	return err
}

// FinishMonitorCheck stores a check's outcome. merge receives the stored row
// inside the transaction, so fields other requests changed during the check
// (an upload job, an upgrade dismissal, a queued recheck) survive it.
func (s *Store) FinishMonitorCheck(v models.MonitorCandidate, merge func(stored models.MonitorCandidate, v *models.MonitorCandidate)) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var body []byte
	if err = tx.QueryRow("SELECT payload FROM monitor_candidates WHERE id=?", v.ID).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil // deleted with its monitor during the check
		}
		return err
	}
	var stored models.MonitorCandidate
	if err = json.Unmarshal(body, &stored); err != nil {
		return err
	}
	merge(stored, &v)
	b, err := encode(v)
	if err != nil {
		return err
	}
	if _, err = tx.Exec("UPDATE monitor_candidates SET status=?,next_check_at=?,payload=? WHERE id=?", v.Status, stamp(v.NextCheckAt), b, v.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteCheckedCandidate removes a candidate the caller claimed for a check.
func (s *Store) DeleteCheckedCandidate(id string) (bool, error) {
	r, err := s.DB.Exec("DELETE FROM monitor_candidates WHERE id=? AND status='checking'", id)
	if err != nil {
		return false, err
	}
	n, err := r.RowsAffected()
	return n == 1, err
}

// ErrCandidateBusy reports a candidate whose state no longer allows a change.
var ErrCandidateBusy = errors.New("candidate is being checked or was changed")

// QueueMonitorCandidate requests a fresh target search for a candidate that
// is not being checked.
func (s *Store) QueueMonitorCandidate(id string, now time.Time) error {
	r, err := s.DB.Exec(`UPDATE monitor_candidates SET status='queued',next_check_at=?,
		payload=json_set(payload,'$.status','queued','$.nextCheckAt',?,'$.message','Queued for a fresh target search')
		WHERE id=? AND status!='checking'`, stamp(now), now.UTC().Format(time.RFC3339Nano), id)
	return s.changedCandidate(r, err, id)
}

// SetCandidateUploadJob links an upload job to a candidate that has none.
func (s *Store) SetCandidateUploadJob(id, jobID string) error {
	r, err := s.DB.Exec(`UPDATE monitor_candidates SET payload=json_set(payload,'$.uploadJobId',?)
		WHERE id=? AND COALESCE(json_extract(payload,'$.uploadJobId'),'')=''`, jobID, id)
	return s.changedCandidate(r, err, id)
}

// SetUpgradeDismissed hides the candidate's upgrade offer when dismissal is
// non-empty, or restores it. offerTitle and offerSize name the offer the
// caller saw; a different current offer is left alone.
func (s *Store) SetUpgradeDismissed(id, dismissal, offerTitle string, offerSize int64) (models.MonitorCandidate, error) {
	guard := ` WHERE id=? AND json_extract(payload,'$.upgradeOffer.title')=? AND json_extract(payload,'$.upgradeOffer.size')=?`
	var r sql.Result
	var err error
	if dismissal == "" {
		r, err = s.DB.Exec(`UPDATE monitor_candidates SET payload=json_remove(payload,'$.upgradeDismissed')`+guard, id, offerTitle, offerSize)
	} else {
		r, err = s.DB.Exec(`UPDATE monitor_candidates SET payload=json_set(payload,'$.upgradeDismissed',?)`+guard, dismissal, id, offerTitle, offerSize)
	}
	if err = s.changedCandidate(r, err, id); err != nil {
		return models.MonitorCandidate{}, err
	}
	return s.MonitorCandidate(id)
}

// changedCandidate turns a guarded update that changed nothing into
// sql.ErrNoRows for a missing candidate, else ErrCandidateBusy.
func (s *Store) changedCandidate(r sql.Result, err error, id string) error {
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil || n == 1 {
		return err
	}
	var exists int
	if err = s.DB.QueryRow("SELECT count(*) FROM monitor_candidates WHERE id=?", id).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return sql.ErrNoRows
	}
	return ErrCandidateBusy
}

func (s *Store) RecoverMonitorCandidates() error {
	_, err := s.DB.Exec("UPDATE monitor_candidates SET status='queued',payload=json_set(payload,'$.status','queued') WHERE status='checking'")
	return err
}

// UpgradeCandidates lists candidates holding an upgrade offer, across
// monitors or for one monitor.
func (s *Store) UpgradeCandidates(monitorID string) ([]models.MonitorCandidate, error) {
	return read[models.MonitorCandidate](s, `SELECT payload FROM monitor_candidates
		WHERE COALESCE(json_extract(payload,'$.upgrade'),'')!='' AND json_type(payload,'$.upgradeOffer')='object' AND (?='' OR monitor_id=?)`, monitorID, monitorID)
}
