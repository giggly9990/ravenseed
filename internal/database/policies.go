package database

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"ravenseed/internal/models"
	"time"
)

func (s *Store) IndexerPolicies() ([]models.IndexerPolicy, error) {
	return read[models.IndexerPolicy](s, "SELECT payload FROM indexer_policies ORDER BY indexer_id")
}

func (s *Store) IndexerPolicy(id int) (models.IndexerPolicy, error) {
	var body []byte
	var v models.IndexerPolicy
	err := s.DB.QueryRow("SELECT payload FROM indexer_policies WHERE indexer_id=?", id).Scan(&body)
	if err != nil {
		return v, err
	}
	return v, json.Unmarshal(body, &v)
}

func (s *Store) SaveIndexerPolicy(v models.IndexerPolicy) (models.IndexerPolicy, error) {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return v, err
	}
	v.SnapshotID = "policy_" + hex.EncodeToString(token[:])
	v.UpdatedAt = time.Now().UTC()
	body, err := encode(v)
	if err != nil {
		return v, err
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return v, err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("INSERT INTO policy_snapshots(id,indexer_id,payload,created_at) VALUES(?,?,?,?)", v.SnapshotID, v.IndexerID, body, stamp(v.UpdatedAt)); err != nil {
		return v, err
	}
	if _, err = tx.Exec("INSERT INTO indexer_policies(indexer_id,snapshot_id,payload) VALUES(?,?,?) ON CONFLICT(indexer_id) DO UPDATE SET snapshot_id=excluded.snapshot_id,payload=excluded.payload", v.IndexerID, v.SnapshotID, body); err != nil {
		return v, err
	}
	return v, tx.Commit()
}

func (s *Store) DeleteIndexerPolicy(id int) (bool, error) {
	r, err := s.DB.Exec("DELETE FROM indexer_policies WHERE indexer_id=?", id)
	if err != nil {
		return false, err
	}
	n, err := r.RowsAffected()
	return n > 0, err
}

func (s *Store) PolicySnapshot(id string) (models.IndexerPolicy, error) {
	var body []byte
	var v models.IndexerPolicy
	err := s.DB.QueryRow("SELECT payload FROM policy_snapshots WHERE id=?", id).Scan(&body)
	if err != nil {
		return v, err
	}
	return v, json.Unmarshal(body, &v)
}

// RequeuePolicyCandidates requests fresh target searches; it leaves the prior
// evaluation visible until each check completes and does not affect uploads.
// A candidate being checked keeps its status but becomes due now; the running
// check keeps that due time, so the candidate is checked again under the
// saved policy.
func (s *Store) RequeuePolicyCandidates(indexerID int, now time.Time) (int64, error) {
	at, payloadAt := stamp(now), now.UTC().Format(time.RFC3339Nano)
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	r, err := tx.Exec(`UPDATE monitor_candidates SET status='queued', next_check_at=?,
		payload=json_set(payload,'$.status','queued','$.nextCheckAt',?,'$.message','Queued for policy re-evaluation')
		WHERE status!='checking' AND monitor_id IN (SELECT id FROM monitors WHERE target_indexer_id=?)`, at, payloadAt, indexerID)
	if err != nil {
		return 0, err
	}
	queued, err := r.RowsAffected()
	if err != nil {
		return 0, err
	}
	r, err = tx.Exec(`UPDATE monitor_candidates SET next_check_at=?, payload=json_set(payload,'$.nextCheckAt',?)
		WHERE status='checking' AND monitor_id IN (SELECT id FROM monitors WHERE target_indexer_id=?)`, at, payloadAt, indexerID)
	if err != nil {
		return 0, err
	}
	checking, err := r.RowsAffected()
	if err != nil {
		return 0, err
	}
	return queued + checking, tx.Commit()
}
