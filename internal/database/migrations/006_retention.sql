CREATE TABLE search_retention (
    search_id TEXT PRIMARY KEY REFERENCES searches(id) ON DELETE CASCADE,
    raw_pruned_at TEXT NOT NULL
);
CREATE INDEX searches_retention ON searches(status, started_at);
