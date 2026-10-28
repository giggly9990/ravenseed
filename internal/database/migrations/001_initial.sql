CREATE TABLE settings (id INTEGER PRIMARY KEY CHECK (id = 1), payload TEXT NOT NULL);
CREATE TABLE searches (id TEXT PRIMARY KEY, cache_key TEXT NOT NULL, started_at TEXT NOT NULL, status TEXT NOT NULL, payload TEXT NOT NULL);
CREATE INDEX searches_cache ON searches(cache_key, started_at DESC);
CREATE TABLE search_indexers (search_id TEXT NOT NULL REFERENCES searches(id) ON DELETE CASCADE, indexer_id INTEGER NOT NULL, payload TEXT NOT NULL, PRIMARY KEY(search_id, indexer_id));
CREATE TABLE search_results (search_id TEXT NOT NULL REFERENCES searches(id) ON DELETE CASCADE, id TEXT NOT NULL, payload TEXT NOT NULL, PRIMARY KEY(search_id, id));
CREATE TABLE release_families (search_id TEXT NOT NULL REFERENCES searches(id) ON DELETE CASCADE, id TEXT NOT NULL, position INTEGER NOT NULL, payload TEXT NOT NULL, PRIMARY KEY(search_id, id));
CREATE TABLE release_family_results (search_id TEXT NOT NULL, family_id TEXT NOT NULL, result_id TEXT NOT NULL, PRIMARY KEY(search_id, family_id, result_id), FOREIGN KEY(search_id, family_id) REFERENCES release_families(search_id, id) ON DELETE CASCADE);
CREATE TABLE size_variants (search_id TEXT NOT NULL, family_id TEXT NOT NULL, position INTEGER NOT NULL, payload TEXT NOT NULL, PRIMARY KEY(search_id, family_id, position), FOREIGN KEY(search_id, family_id) REFERENCES release_families(search_id, id) ON DELETE CASCADE);
