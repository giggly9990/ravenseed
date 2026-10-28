package database

import (
	"encoding/json"
	"errors"
	"time"
)

func metadataTable(season bool) string {
	if season {
		return "season_metadata"
	}
	return "media_metadata"
}

func (s *Store) Metadata(key string, season bool, out any) (time.Time, error) {
	var payload []byte
	var expires string
	err := s.DB.QueryRow("SELECT payload,expires_at FROM "+metadataTable(season)+" WHERE cache_key=?", key).Scan(&payload, &expires)
	if err != nil {
		return time.Time{}, err
	}
	until, err := parseStamp(expires)
	if err != nil {
		return until, err
	}
	return until, json.Unmarshal(payload, out)
}

func (s *Store) SaveMetadata(key string, season bool, value any, raw json.RawMessage, now, expires time.Time) error {
	if !json.Valid(raw) {
		return errors.New("invalid metadata JSON")
	}
	b, err := encode(value)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec("INSERT INTO "+metadataTable(season)+"(cache_key,payload,metadata_json,fetched_at,expires_at) VALUES (?,?,?,?,?) ON CONFLICT(cache_key) DO UPDATE SET payload=excluded.payload,metadata_json=excluded.metadata_json,fetched_at=excluded.fetched_at,expires_at=excluded.expires_at", key, b, string(raw), stamp(now), stamp(expires))
	return err
}
