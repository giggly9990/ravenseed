package database

import (
	"ravenseed/internal/models"
)

func (s *Store) Presets() ([]models.IndexerPreset, error) {
	return read[models.IndexerPreset](s, "SELECT payload FROM indexer_presets ORDER BY name")
}

func (s *Store) SavePreset(p models.IndexerPreset) error {
	b, err := encode(p)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec("INSERT INTO indexer_presets(id,name,payload) VALUES (?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,payload=excluded.payload", p.ID, p.Name, b)
	return err
}

// DeletePreset reports whether the preset existed.
func (s *Store) DeletePreset(id string) (bool, error) {
	r, err := s.DB.Exec("DELETE FROM indexer_presets WHERE id=?", id)
	if err != nil {
		return false, err
	}
	n, err := r.RowsAffected()
	return n > 0, err
}
