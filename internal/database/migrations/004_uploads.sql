CREATE TABLE upload_settings (id INTEGER PRIMARY KEY CHECK(id=1), payload BLOB NOT NULL);
CREATE TABLE upload_jobs (
    id TEXT PRIMARY KEY,
    search_id TEXT NOT NULL,
    result_id TEXT NOT NULL,
    instance_id INTEGER NOT NULL,
    target_id INTEGER NOT NULL,
    status TEXT NOT NULL,
    created_at TEXT NOT NULL,
    payload BLOB NOT NULL,
    UNIQUE(search_id, result_id, instance_id, target_id)
);
CREATE TABLE upload_claims (
    connection TEXT NOT NULL,
    instance_id INTEGER NOT NULL,
    hash TEXT NOT NULL,
    tracker TEXT NOT NULL,
    job_id TEXT NOT NULL,
    PRIMARY KEY(connection, instance_id, hash, tracker)
);
