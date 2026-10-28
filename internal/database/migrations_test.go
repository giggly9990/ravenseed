package database

import (
	"database/sql"
	"encoding/json"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"ravenseed/internal/models"
)

// Every migration after the two that shipped as 002 has its own prefix.
func TestMigrationPrefixesAreUnique(t *testing.T) {
	entries, err := fs.ReadDir(migrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		prefix, _, ok := strings.Cut(entry.Name(), "_")
		if !ok || len(prefix) != 3 {
			t.Fatalf("migration %s lacks a three-digit prefix", entry.Name())
		}
		if previous, ok := seen[prefix]; ok && prefix != "002" {
			t.Fatalf("migrations %s and %s share a prefix", previous, entry.Name())
		}
		seen[prefix] = entry.Name()
	}
}

// Rows written before 010 used variable-width times and BLOB payloads.
func TestFixedWidthTimeMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "times.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("CREATE TABLE schema_migrations(version TEXT PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	entries, err := fs.ReadDir(migrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".sql" || entry.Name() >= "010" {
			continue
		}
		body, err := fs.ReadFile(migrations, "migrations/"+entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(string(body)); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec("INSERT INTO schema_migrations VALUES (?)", entry.Name()); err != nil {
			t.Fatal(err)
		}
	}
	job := func(id string) []byte {
		b, _ := json.Marshal(models.Job{ID: id, Status: "completed"})
		return b
	}
	for id, started := range map[string]string{"whole": "2026-01-01T00:00:05Z", "short": "2026-01-01T00:00:05.1Z", "long": "2026-01-01T00:00:05.123456789Z"} {
		if _, err = db.Exec("INSERT INTO searches VALUES (?,?,?,?,?)", id, "k", started, "completed", job(id)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec(`INSERT INTO monitors VALUES ('m',1,'a',2,1,?)`, []byte(`{"id":"m","enabled":true}`)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO monitor_candidates VALUES ('c','m',1,'h','queued','2026-01-01T00:00:05.5Z',?)`, []byte(`{"id":"c","monitorId":"m","status":"queued"}`)); err != nil {
		t.Fatal(err)
	}
	db.Close()
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	got := map[string]string{}
	rows, err := store.DB.Query("SELECT id, started_at, typeof(payload) FROM searches")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id, started, kind string
		if err = rows.Scan(&id, &started, &kind); err != nil {
			t.Fatal(err)
		}
		if kind != "text" {
			t.Fatalf("%s payload stored as %s", id, kind)
		}
		got[id] = started
	}
	rows.Close()
	want := map[string]string{"whole": "2026-01-01T00:00:05.000000000Z", "short": "2026-01-01T00:00:05.100000000Z", "long": "2026-01-01T00:00:05.123456789Z"}
	for id, value := range want {
		if got[id] != value {
			t.Fatalf("%s: %q, want %q", id, got[id], value)
		}
	}
	var next, kind string
	if err = store.DB.QueryRow("SELECT next_check_at, typeof(payload) FROM monitor_candidates").Scan(&next, &kind); err != nil || next != "2026-01-01T00:00:05.500000000Z" || kind != "text" {
		t.Fatalf("candidate: %q %q %v", next, kind, err)
	}
	// Ordering by the column now matches time order.
	history, _, err := store.History(10, 0)
	if err != nil || len(history) != 3 || history[0].ID != "long" || history[2].ID != "whole" {
		t.Fatalf("history order: %+v %v", history, err)
	}
	if v, err := store.DueMonitorCandidate(time.Date(2026, 1, 1, 0, 0, 5, 600000000, time.UTC)); err != nil || v.ID != "c" {
		t.Fatalf("due candidate: %+v %v", v, err)
	}
	if _, err := store.DueMonitorCandidate(time.Date(2026, 1, 1, 0, 0, 5, 400000000, time.UTC)); err == nil {
		t.Fatal("candidate due early")
	}
}

func TestUpgradeEveryShippedSchema(t *testing.T) {
	entries, err := fs.ReadDir(migrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	for prefix := 0; prefix <= len(entries); prefix++ {
		t.Run(strconv.Itoa(prefix), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "upgrade.db")
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec("CREATE TABLE schema_migrations(version TEXT PRIMARY KEY)"); err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries[:prefix] {
				body, err := fs.ReadFile(migrations, "migrations/"+entry.Name())
				if err != nil {
					t.Fatal(err)
				}
				if _, err = db.Exec(string(body)); err != nil {
					t.Fatal(err)
				}
				if _, err = db.Exec("INSERT INTO schema_migrations VALUES (?)", entry.Name()); err != nil {
					t.Fatal(err)
				}
			}
			if prefix > 0 {
				if _, err = db.Exec(`INSERT INTO settings VALUES (1,'{"quiUrl":"http://saved","apiKey":"keep-me"}')`); err != nil {
					t.Fatal(err)
				}
			}
			db.Close()
			for reopen := 0; reopen < 2; reopen++ {
				store, err := Open(path)
				if err != nil {
					t.Fatal(err)
				}
				var count int
				if err = store.DB.QueryRow("SELECT count(*) FROM schema_migrations").Scan(&count); err != nil || count != len(entries) {
					t.Fatalf("migrations: %d %v", count, err)
				}
				if prefix > 0 {
					settings, err := store.Settings()
					if err != nil || settings.APIKey != "keep-me" {
						t.Fatal("upgrade lost saved settings", err)
					}
				}
				for _, table := range []string{"searches", "search_events", "indexer_presets", "media_metadata", "upload_jobs", "torrent_settings", "search_retention"} {
					if _, err = store.DB.Exec("SELECT * FROM " + table + " LIMIT 0"); err != nil {
						t.Fatal(table, err)
					}
				}
				store.DB.Close()
			}
		})
	}
}

type unreadableMigration struct{ fs.FS }

func (f unreadableMigration) ReadFile(string) ([]byte, error) { return nil, fs.ErrPermission }

func TestMigrationErrorsRollBack(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	broken := fstest.MapFS{"migrations/999_failure.sql": {Data: []byte("CREATE TABLE should_rollback(id TEXT); THIS IS NOT SQL;")}}
	if err := applyMigrations(store.DB, broken); err == nil {
		t.Fatal("invalid migration accepted")
	}
	var count int
	store.DB.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='should_rollback'").Scan(&count)
	if count != 0 {
		t.Fatal("partial migration persisted")
	}
	store.DB.QueryRow("SELECT count(*) FROM schema_migrations WHERE version='999_failure.sql'").Scan(&count)
	if count != 0 {
		t.Fatal("failed migration recorded")
	}
	if err := applyMigrations(store.DB, fstest.MapFS{}); err == nil {
		t.Fatal("missing migration directory ignored")
	}
	if err := applyMigrations(store.DB, unreadableMigration{broken}); err == nil {
		t.Fatal("unreadable migration ignored")
	}
}
