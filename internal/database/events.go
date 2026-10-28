package database

import (
	"encoding/json"
	"ravenseed/internal/models"
)

type Event struct {
	ID   int64           `json:"id"`
	Name string          `json:"name"`
	Data json.RawMessage `json:"data"`
}

func (s *Store) Event(name string, j models.Job) error {
	b, err := encode(j)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec("INSERT INTO search_events(search_id,name,payload) VALUES(?,?,?)", j.ID, name, b)
	return err
}
func (s *Store) Events(id string, after int64) ([]Event, error) {
	rows, err := s.DB.Query("SELECT id,name,payload FROM search_events WHERE search_id=? AND id>? ORDER BY id LIMIT 100", id, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		var data []byte // TEXT does not scan into json.RawMessage directly
		if err = rows.Scan(&e.ID, &e.Name, &data); err != nil {
			return nil, err
		}
		e.Data = data
		out = append(out, e)
	}
	return out, rows.Err()
}
