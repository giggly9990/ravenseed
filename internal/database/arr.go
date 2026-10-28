package database

import (
	"database/sql"
	"encoding/json"
	"ravenseed/internal/models"
	"strings"
	"time"
)

func (s *Store) ArrSettings() (models.ArrSettings, error) {
	v := models.ArrSettings{Instances: []models.ArrInstance{}}
	var b []byte
	err := s.DB.QueryRow("SELECT payload FROM arr_settings WHERE id=1").Scan(&b)
	if err == sql.ErrNoRows {
		return v, nil
	}
	if err != nil {
		return v, err
	}
	if err = json.Unmarshal(b, &v); v.Instances == nil {
		v.Instances = []models.ArrInstance{}
	}
	return v, err
}

// SaveArrSettings also clears cached torrent identities: they may have come
// from an instance that was removed or changed.
func (s *Store) SaveArrSettings(v models.ArrSettings) error {
	b, err := encode(v)
	if err != nil {
		return err
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("INSERT INTO arr_settings(id,payload) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload", b); err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM torrent_identities"); err != nil {
		return err
	}
	return tx.Commit()
}

// TorrentIdentity returns an unexpired cached identity for a torrent hash.
func (s *Store) TorrentIdentity(hash string, now time.Time) (models.TorrentIdentity, error) {
	var v models.TorrentIdentity
	var b []byte
	err := s.DB.QueryRow("SELECT payload FROM torrent_identities WHERE hash=? AND expires_at>?", strings.ToLower(hash), stamp(now)).Scan(&b)
	if err != nil {
		return v, err
	}
	return v, json.Unmarshal(b, &v)
}

func (s *Store) SaveTorrentIdentity(v models.TorrentIdentity) error {
	v.Hash = strings.ToLower(v.Hash)
	b, err := encode(v)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec("INSERT INTO torrent_identities(hash,expires_at,payload) VALUES(?,?,?) ON CONFLICT(hash) DO UPDATE SET expires_at=excluded.expires_at,payload=excluded.payload", v.Hash, stamp(v.ExpiresAt), b)
	return err
}

func (s *Store) DeleteTorrentIdentity(hash string) error {
	_, err := s.DB.Exec("DELETE FROM torrent_identities WHERE hash=?", strings.ToLower(hash))
	return err
}

func (s *Store) PruneTorrentIdentities(now time.Time) error {
	_, err := s.DB.Exec("DELETE FROM torrent_identities WHERE expires_at<=?", stamp(now))
	return err
}
