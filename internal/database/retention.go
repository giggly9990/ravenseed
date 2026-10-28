package database

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// metadataStaleFor keeps expired TMDb metadata as a fallback for outages.
const metadataStaleFor = 90 * 24 * time.Hour

// Prune removes expired history and raw responses in small transactions, so
// other requests are not held up for the whole run. Expired metadata is
// removed whatever the retention settings.
func (s *Store) Prune(ctx context.Context, now time.Time, history, raw time.Duration) error {
	if history > 0 {
		cutoff := stamp(now.Add(-history))
		_, err := s.DB.ExecContext(ctx, `DELETE FROM searches WHERE id IN (
   SELECT id FROM searches WHERE status != 'running' AND started_at < ?
   AND NOT EXISTS (SELECT 1 FROM upload_jobs WHERE search_id=searches.id AND status IN ('adding','downloading','uploading','needs_review'))
   ORDER BY started_at LIMIT 100)`, cutoff)
		if err != nil {
			return err
		}
		if err = s.pruneUploads(ctx, cutoff); err != nil {
			return err
		}
	}
	if raw > 0 {
		rows, err := s.DB.QueryContext(ctx, `SELECT id FROM searches WHERE status != 'running' AND started_at < ?
   AND NOT EXISTS (SELECT 1 FROM search_retention WHERE search_id=searches.id) ORDER BY started_at LIMIT 100`, stamp(now.Add(-raw)))
		if err != nil {
			return err
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err := s.pruneRaw(ctx, id, now); err != nil {
				return err
			}
		}
	}
	for _, table := range []string{"media_metadata", "season_metadata"} {
		if _, err := s.DB.ExecContext(ctx, "DELETE FROM "+table+" WHERE expires_at < ?", stamp(now.Add(-metadataStaleFor))); err != nil {
			return err
		}
	}
	return nil
}

// uploadEnded is when an upload job last changed, its end once settled. The
// payload's updatedAt is RFC 3339 of varying width, so times compare as days.
const uploadEnded = "max(julianday(created_at),COALESCE(julianday(json_extract(payload,'$.updatedAt')),0)) < julianday(?)"

// pruneUploads removes upload jobs settled before the cutoff. A finished
// job's claim stays, so its torrent is not submitted twice. A failed or
// cancelled job could have been reuploaded, so its claim and monitor candidate
// link go with it, and the candidate can be queued again.
func (s *Store) pruneUploads(ctx context.Context, cutoff string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	released := "SELECT id FROM upload_jobs WHERE status IN ('failed','cancelled') AND " + uploadEnded
	if _, err = tx.ExecContext(ctx, `UPDATE monitor_candidates SET payload=json_remove(payload,'$.uploadJobId')
		WHERE json_extract(payload,'$.uploadJobId') IN (`+released+`)`, cutoff); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM upload_claims WHERE job_id IN ("+released+")", cutoff); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM upload_jobs WHERE status IN ('finished','failed','cancelled') AND "+uploadEnded, cutoff); err != nil {
		return err
	}
	return tx.Commit()
}

// pruneRaw strips raw upstream responses from one search in one transaction.
func (s *Store) pruneRaw(ctx context.Context, id string, now time.Time) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, table := range []string{"search_results", "release_families", "size_variants"} {
		rows, err := tx.QueryContext(ctx, "SELECT rowid,payload FROM "+table+" WHERE search_id=?", id)
		if err != nil {
			return err
		}
		type record struct {
			id    int64
			value []byte
		}
		var records []record
		for rows.Next() {
			var r record
			if err := rows.Scan(&r.id, &r.value); err != nil {
				rows.Close()
				return err
			}
			records = append(records, r)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, record := range records {
			var value any
			decoder := json.NewDecoder(bytes.NewReader(record.value))
			decoder.UseNumber()
			if err := decoder.Decode(&value); err != nil {
				return fmt.Errorf("invalid retained payload: %w", err)
			}
			stripRaw(value)
			body, err := encode(value)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, "UPDATE "+table+" SET payload=? WHERE rowid=?", body, record.id); err != nil {
				return err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO search_retention(search_id,raw_pruned_at) VALUES (?,?)", id, stamp(now)); err != nil {
		return err
	}
	return tx.Commit()
}

func stripRaw(value any) {
	switch value := value.(type) {
	case map[string]any:
		if raw, ok := value["raw"].(map[string]any); ok {
			for normalized, upstream := range map[string]string{"downloadFactor": "download_volume_factor", "uploadFactor": "upload_volume_factor"} {
				if factor, ok := raw[upstream].(json.Number); ok {
					value[normalized] = factor
				} else {
					delete(value, normalized)
				}
			}
		}
		delete(value, "raw")
		for _, child := range value {
			stripRaw(child)
		}
	case []any:
		for _, child := range value {
			stripRaw(child)
		}
	}
}
