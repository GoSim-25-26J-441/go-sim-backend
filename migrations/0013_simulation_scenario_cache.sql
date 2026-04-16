CREATE TABLE IF NOT EXISTS simulation_scenario_cache (
    diagram_version_id TEXT PRIMARY KEY
        REFERENCES diagram_versions(id)
        ON DELETE CASCADE,
    scenario_yaml TEXT NOT NULL,
    scenario_hash TEXT NOT NULL,
    s3_path TEXT,
    source TEXT NOT NULL DEFAULT 'request',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_simulation_scenario_cache_hash
ON simulation_scenario_cache(scenario_hash);
