CREATE TABLE monitors (
    id TEXT PRIMARY KEY,
    source_indexer_id INTEGER NOT NULL,
    source_tracker_host TEXT NOT NULL,
    target_indexer_id INTEGER NOT NULL,
    enabled INTEGER NOT NULL,
    payload BLOB NOT NULL,
    UNIQUE(source_indexer_id, source_tracker_host, target_indexer_id)
);
CREATE TABLE monitor_candidates (
    id TEXT PRIMARY KEY,
    monitor_id TEXT NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
    instance_id INTEGER NOT NULL,
    hash TEXT NOT NULL,
    status TEXT NOT NULL,
    next_check_at TEXT NOT NULL,
    payload BLOB NOT NULL,
    UNIQUE(monitor_id, instance_id, hash)
);
CREATE INDEX monitor_candidates_due ON monitor_candidates(status, next_check_at);
