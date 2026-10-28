CREATE TABLE arr_settings (id INTEGER PRIMARY KEY CHECK(id=1), payload BLOB NOT NULL);
CREATE TABLE torrent_identities (
    hash TEXT PRIMARY KEY,
    expires_at TEXT NOT NULL,
    payload BLOB NOT NULL
);
CREATE INDEX torrent_identities_expiry ON torrent_identities(expires_at);
