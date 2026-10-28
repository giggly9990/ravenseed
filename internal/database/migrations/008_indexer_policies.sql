CREATE TABLE indexer_policies (
    indexer_id INTEGER PRIMARY KEY,
    snapshot_id TEXT NOT NULL,
    payload BLOB NOT NULL
);
CREATE TABLE policy_snapshots (
    id TEXT PRIMARY KEY,
    indexer_id INTEGER NOT NULL,
    payload BLOB NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX policy_snapshots_indexer ON policy_snapshots(indexer_id, created_at);
