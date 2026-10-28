CREATE TABLE media_metadata (
    cache_key TEXT PRIMARY KEY,
    payload BLOB NOT NULL,
    metadata_json BLOB NOT NULL,
    fetched_at TEXT NOT NULL,
    expires_at TEXT NOT NULL
);
CREATE TABLE season_metadata (
    cache_key TEXT PRIMARY KEY,
    payload BLOB NOT NULL,
    metadata_json BLOB NOT NULL,
    fetched_at TEXT NOT NULL,
    expires_at TEXT NOT NULL
);
