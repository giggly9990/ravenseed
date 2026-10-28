package database

import (
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	_ "modernc.org/sqlite"
	"ravenseed/internal/models"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct{ DB *sql.DB }

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	if f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0600); err == nil {
		f.Close()
	}
	for _, file := range []string{path, path + "-wal", path + "-shm"} {
		if err := restrictFile(file); err != nil {
			return nil, err
		}
	}
	// Pragmas in the DSN apply to every connection the pool opens.
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db}
	if _, err = db.Exec("CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY)"); err != nil {
		db.Close()
		return nil, err
	}
	if err := applyMigrations(db, migrations); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func applyMigrations(db *sql.DB, source fs.FS) error {
	entries, err := fs.ReadDir(source, "migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		var count int
		if err = db.QueryRow("SELECT count(*) FROM schema_migrations WHERE version=?", entry.Name()).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		body, err := fs.ReadFile(source, "migrations/"+entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		tx, e := db.Begin()
		if e != nil {
			return e
		}
		if _, e = tx.Exec(string(body)); e == nil {
			_, e = tx.Exec("INSERT INTO schema_migrations VALUES (?)", entry.Name())
		}
		if e != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", entry.Name(), e)
		}
		if e = tx.Commit(); e != nil {
			return e
		}
	}
	return nil
}

func (s *Store) Settings() (models.Settings, error) {
	var v models.Settings
	var b []byte
	err := s.DB.QueryRow("SELECT payload FROM settings WHERE id=1").Scan(&b)
	if err == sql.ErrNoRows {
		return v, nil
	}
	if err != nil {
		return v, err
	}
	return v, json.Unmarshal(b, &v)
}
func (s *Store) SaveSettings(v models.Settings) error {
	b, err := encode(v)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec("INSERT INTO settings(id,payload) VALUES (1,?) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload", b)
	return err
}
func (s *Store) SaveJob(j models.Job, key string) error {
	b, err := encode(j)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec("INSERT INTO searches(id,cache_key,started_at,status,payload) VALUES (?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET status=excluded.status,payload=excluded.payload", j.ID, key, stamp(j.StartedAt), j.Status, b)
	return err
}
func (s *Store) Job(id string) (models.Job, error) {
	var j models.Job
	var b []byte
	err := s.DB.QueryRow("SELECT payload FROM searches WHERE id=?", id).Scan(&b)
	if err != nil {
		return j, err
	}
	return j, json.Unmarshal(b, &j)
}
func (s *Store) Cached(key string, since time.Time) (models.Job, error) {
	var j models.Job
	var b []byte
	err := s.DB.QueryRow("SELECT payload FROM searches WHERE cache_key=? AND status='completed' AND started_at>? ORDER BY started_at DESC LIMIT 1", key, stamp(since)).Scan(&b)
	if err != nil {
		return j, err
	}
	err = json.Unmarshal(b, &j)
	if len(j.IndexerErrors) > 0 {
		return j, sql.ErrNoRows
	}
	j.CacheHit = true
	return j, err
}
func (s *Store) History(limit, offset int) ([]models.Job, int, error) {
	var total int
	if err := s.DB.QueryRow("SELECT count(*) FROM searches").Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.Query("SELECT payload FROM searches ORDER BY started_at DESC LIMIT ? OFFSET ?", limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []models.Job{}
	for rows.Next() {
		var b []byte
		var j models.Job
		if err = rows.Scan(&b); err != nil {
			return nil, 0, err
		}
		if err = json.Unmarshal(b, &j); err != nil {
			return nil, 0, err
		}
		out = append(out, j)
	}
	return out, total, rows.Err()
}
func (s *Store) Recover() error {
	rows, err := s.DB.Query("SELECT payload FROM searches WHERE status='running'")
	if err != nil {
		return err
	}
	var jobs []models.Job
	for rows.Next() {
		var b []byte
		var j models.Job
		if err = rows.Scan(&b); err != nil {
			rows.Close()
			return err
		}
		if err = json.Unmarshal(b, &j); err != nil {
			rows.Close()
			return err
		}
		jobs = append(jobs, j)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, j := range jobs {
		j.Status = "failed"
		j.Error = "Service restarted before completion"
		now := time.Now().UTC()
		j.CompletedAt = &now
		for i := range j.Indexers {
			if j.Indexers[i].Status == "running" || j.Indexers[i].Status == "queued" {
				j.Indexers[i].Status = "failed"
				j.Indexers[i].Error = j.Error
			}
		}
		if err = s.SaveJob(j, ""); err != nil {
			return err
		}
	}
	return nil
}

// storedResult encodes a search result with its download link and full raw
// response, which models.Result.MarshalJSON leaves out of API responses.
// Adding a torrent reads the link back from search_results.
type storedResult models.Result

// storedFamilyResults strips the raw upstream response and parser output
// from a family's results: both are stored once, with the search result.
func storedFamilyResults(f models.Family) models.Family {
	f.Results = slices.Clone(f.Results)
	for i := range f.Results {
		f.Results[i].Raw, f.Results[i].ParserOutput = nil, nil
	}
	return f
}

func (s *Store) SaveAnalysis(j models.Job, results []models.Result, families []models.Family) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	insert := func(query string, args ...any) error { _, e := tx.Exec(query, args...); return e }
	for _, v := range j.Indexers {
		b, err := encode(v)
		if err != nil {
			return err
		}
		if err = insert("INSERT INTO search_indexers(search_id,indexer_id,payload) VALUES (?,?,?)", j.ID, v.IndexerID, b); err != nil {
			return err
		}
	}
	for _, v := range results {
		b, err := encode(storedResult(v))
		if err != nil {
			return err
		}
		if err = insert("INSERT INTO search_results(search_id,id,payload) VALUES (?,?,?)", j.ID, v.ID, b); err != nil {
			return err
		}
	}
	for i, v := range families {
		b, err := encode(storedFamilyResults(v))
		if err != nil {
			return err
		}
		if err = insert("INSERT INTO release_families(search_id,id,position,payload) VALUES (?,?,?,?)", j.ID, v.ID, i, b); err != nil {
			return err
		}
		for _, r := range v.Results {
			if err = insert("INSERT INTO release_family_results(search_id,family_id,result_id) VALUES (?,?,?)", j.ID, v.ID, r.ID); err != nil {
				return err
			}
		}
		for k, variant := range v.Variants {
			if b, err = encode(variant); err != nil {
				return err
			}
			if err = insert("INSERT INTO size_variants(search_id,family_id,position,payload) VALUES (?,?,?,?)", j.ID, v.ID, k, b); err != nil {
				return err
			}
		}
	}
	b, err := encode(j)
	if err != nil {
		return err
	}
	if err = insert("UPDATE searches SET status=?,payload=? WHERE id=?", j.Status, b, j.ID); err != nil {
		return err
	}
	if err = insert("INSERT INTO search_events(search_id,name,payload) VALUES(?,?,?)", j.ID, "search.completed", b); err != nil {
		return err
	}
	return tx.Commit()
}
func read[T any](s *Store, query string, args ...any) ([]T, error) {
	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []T{}
	for rows.Next() {
		var b []byte
		var v T
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(b, &v); err != nil {
			return nil, fmt.Errorf("decode stored result: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) Results(id string) ([]models.Result, error) {
	return read[models.Result](s, "SELECT payload FROM search_results WHERE search_id=? ORDER BY id", id)
}
func emptyJSON(v json.RawMessage) bool { return len(v) == 0 || string(v) == "null" }

// Families restores each family result's raw response and parser output from
// the search's stored results.
func (s *Store) Families(id string) ([]models.Family, error) {
	families, err := read[models.Family](s, "SELECT payload FROM release_families WHERE search_id=? ORDER BY position", id)
	if err != nil {
		return nil, err
	}
	var byID map[string]models.Result
	for i := range families {
		for k := range families[i].Results {
			r := &families[i].Results[k]
			if !emptyJSON(r.Raw) || !emptyJSON(r.ParserOutput) {
				continue // stored before results were shared, or already complete
			}
			if byID == nil {
				results, err := s.Results(id)
				if err != nil {
					return nil, err
				}
				byID = make(map[string]models.Result, len(results))
				for _, v := range results {
					byID[v.ID] = v
				}
			}
			if v, ok := byID[r.ID]; ok {
				r.Raw, r.ParserOutput = v.Raw, v.ParserOutput
			}
		}
	}
	return families, nil
}
