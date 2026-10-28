package database

import (
	"database/sql"
	"encoding/json"
	"ravenseed/internal/models"
)

func (s *Store) TorrentSettings() (models.TorrentSettings, error) {
	v := models.TorrentSettings{Categories: []models.InstanceCategories{}}
	var b []byte
	err := s.DB.QueryRow("SELECT payload FROM torrent_settings WHERE id=1").Scan(&b)
	if err == sql.ErrNoRows {
		return v, nil
	}
	if err != nil {
		return v, err
	}
	return v, json.Unmarshal(b, &v)
}

func (s *Store) SaveTorrentSettings(v models.TorrentSettings) error {
	b, err := encode(v)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec("INSERT INTO torrent_settings(id,payload) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload", b)
	return err
}
