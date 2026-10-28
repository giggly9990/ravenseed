-- Time columns move to one fixed-width UTC layout,
-- 2006-01-02T15:04:05.000000000Z, so they compare and sort as strings.
-- RFC 3339 values written before trimmed trailing zeros from the fraction.
UPDATE searches SET started_at = substr(started_at,1,19) || '.' || substr(CASE WHEN substr(started_at,20,1)='.' THEN substr(started_at,21,length(started_at)-21) ELSE '' END || '000000000',1,9) || 'Z'
    WHERE length(started_at) != 30 AND started_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]*Z';
UPDATE monitor_candidates SET next_check_at = substr(next_check_at,1,19) || '.' || substr(CASE WHEN substr(next_check_at,20,1)='.' THEN substr(next_check_at,21,length(next_check_at)-21) ELSE '' END || '000000000',1,9) || 'Z'
    WHERE length(next_check_at) != 30 AND next_check_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]*Z';
UPDATE upload_jobs SET created_at = substr(created_at,1,19) || '.' || substr(CASE WHEN substr(created_at,20,1)='.' THEN substr(created_at,21,length(created_at)-21) ELSE '' END || '000000000',1,9) || 'Z'
    WHERE length(created_at) != 30 AND created_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]*Z';
UPDATE torrent_identities SET expires_at = substr(expires_at,1,19) || '.' || substr(CASE WHEN substr(expires_at,20,1)='.' THEN substr(expires_at,21,length(expires_at)-21) ELSE '' END || '000000000',1,9) || 'Z'
    WHERE length(expires_at) != 30 AND expires_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]*Z';
UPDATE policy_snapshots SET created_at = substr(created_at,1,19) || '.' || substr(CASE WHEN substr(created_at,20,1)='.' THEN substr(created_at,21,length(created_at)-21) ELSE '' END || '000000000',1,9) || 'Z'
    WHERE length(created_at) != 30 AND created_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]*Z';
UPDATE media_metadata SET fetched_at = substr(fetched_at,1,19) || '.' || substr(CASE WHEN substr(fetched_at,20,1)='.' THEN substr(fetched_at,21,length(fetched_at)-21) ELSE '' END || '000000000',1,9) || 'Z'
    WHERE length(fetched_at) != 30 AND fetched_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]*Z';
UPDATE media_metadata SET expires_at = substr(expires_at,1,19) || '.' || substr(CASE WHEN substr(expires_at,20,1)='.' THEN substr(expires_at,21,length(expires_at)-21) ELSE '' END || '000000000',1,9) || 'Z'
    WHERE length(expires_at) != 30 AND expires_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]*Z';
UPDATE season_metadata SET fetched_at = substr(fetched_at,1,19) || '.' || substr(CASE WHEN substr(fetched_at,20,1)='.' THEN substr(fetched_at,21,length(fetched_at)-21) ELSE '' END || '000000000',1,9) || 'Z'
    WHERE length(fetched_at) != 30 AND fetched_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]*Z';
UPDATE season_metadata SET expires_at = substr(expires_at,1,19) || '.' || substr(CASE WHEN substr(expires_at,20,1)='.' THEN substr(expires_at,21,length(expires_at)-21) ELSE '' END || '000000000',1,9) || 'Z'
    WHERE length(expires_at) != 30 AND expires_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]*Z';
UPDATE search_retention SET raw_pruned_at = substr(raw_pruned_at,1,19) || '.' || substr(CASE WHEN substr(raw_pruned_at,20,1)='.' THEN substr(raw_pruned_at,21,length(raw_pruned_at)-21) ELSE '' END || '000000000',1,9) || 'Z'
    WHERE length(raw_pruned_at) != 30 AND raw_pruned_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]*Z';
-- JSON payloads were bound as BLOBs, which SQLite's JSON functions read as
-- binary JSONB; store them as text.
UPDATE settings SET payload = CAST(payload AS TEXT) WHERE typeof(payload) = 'blob';
UPDATE searches SET payload = CAST(payload AS TEXT) WHERE typeof(payload) = 'blob';
UPDATE search_indexers SET payload = CAST(payload AS TEXT) WHERE typeof(payload) = 'blob';
UPDATE search_results SET payload = CAST(payload AS TEXT) WHERE typeof(payload) = 'blob';
UPDATE release_families SET payload = CAST(payload AS TEXT) WHERE typeof(payload) = 'blob';
UPDATE size_variants SET payload = CAST(payload AS TEXT) WHERE typeof(payload) = 'blob';
UPDATE search_events SET payload = CAST(payload AS TEXT) WHERE typeof(payload) = 'blob';
UPDATE indexer_presets SET payload = CAST(payload AS TEXT) WHERE typeof(payload) = 'blob';
UPDATE media_metadata SET payload = CAST(payload AS TEXT) WHERE typeof(payload) = 'blob';
UPDATE media_metadata SET metadata_json = CAST(metadata_json AS TEXT) WHERE typeof(metadata_json) = 'blob';
UPDATE season_metadata SET payload = CAST(payload AS TEXT) WHERE typeof(payload) = 'blob';
UPDATE season_metadata SET metadata_json = CAST(metadata_json AS TEXT) WHERE typeof(metadata_json) = 'blob';
UPDATE upload_settings SET payload = CAST(payload AS TEXT) WHERE typeof(payload) = 'blob';
UPDATE upload_jobs SET payload = CAST(payload AS TEXT) WHERE typeof(payload) = 'blob';
UPDATE torrent_settings SET payload = CAST(payload AS TEXT) WHERE typeof(payload) = 'blob';
UPDATE monitors SET payload = CAST(payload AS TEXT) WHERE typeof(payload) = 'blob';
UPDATE monitor_candidates SET payload = CAST(payload AS TEXT) WHERE typeof(payload) = 'blob';
UPDATE indexer_policies SET payload = CAST(payload AS TEXT) WHERE typeof(payload) = 'blob';
UPDATE policy_snapshots SET payload = CAST(payload AS TEXT) WHERE typeof(payload) = 'blob';
UPDATE arr_settings SET payload = CAST(payload AS TEXT) WHERE typeof(payload) = 'blob';
UPDATE torrent_identities SET payload = CAST(payload AS TEXT) WHERE typeof(payload) = 'blob';
-- Indexes for ordered listings and the monitor queue.
CREATE INDEX searches_started ON searches(started_at);
CREATE INDEX upload_jobs_created ON upload_jobs(created_at);
CREATE INDEX upload_jobs_status ON upload_jobs(status, created_at);
CREATE INDEX monitor_candidates_waiting ON monitor_candidates(next_check_at) WHERE status != 'checking';
