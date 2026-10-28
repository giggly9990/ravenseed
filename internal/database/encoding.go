package database

import (
	"encoding/json"
	"os"
	"time"
)

// timeLayout is the fixed-width UTC layout of every time column, so stored
// times compare and sort correctly as strings. Reads also accept the older
// variable-width RFC 3339 values.
const timeLayout = "2006-01-02T15:04:05.000000000Z"

func stamp(t time.Time) string { return t.UTC().Format(timeLayout) }

func parseStamp(value string) (time.Time, error) { return time.Parse(time.RFC3339Nano, value) }

// encode marshals a payload for binding as TEXT: SQLite's JSON functions read
// BLOB arguments as binary JSONB.
func encode(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

// restrictFile limits an existing database file to its owner. SQLite gives
// the -wal and -shm files the database file's permissions.
func restrictFile(path string) error {
	if err := os.Chmod(path, 0600); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
