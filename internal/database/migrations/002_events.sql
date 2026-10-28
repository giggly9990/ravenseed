CREATE TABLE search_events (id INTEGER PRIMARY KEY AUTOINCREMENT, search_id TEXT NOT NULL REFERENCES searches(id) ON DELETE CASCADE, name TEXT NOT NULL, payload TEXT NOT NULL);
CREATE INDEX search_events_job ON search_events(search_id, id);
