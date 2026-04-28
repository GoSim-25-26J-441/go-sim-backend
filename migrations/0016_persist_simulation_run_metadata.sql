-- Persist simulation run metadata in PostgreSQL.
--
-- Redis is still used for short-lived cache entries and Pub/Sub events, but
-- run identity, ownership, status, and engine mapping must survive Redis or
-- process restarts.

ALTER TABLE simulation_runs
ADD COLUMN IF NOT EXISTS user_id TEXT,
ADD COLUMN IF NOT EXISTS project_public_id TEXT,
ADD COLUMN IF NOT EXISTS engine_run_id TEXT,
ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'pending',
ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
ADD COLUMN IF NOT EXISTS completed_at TIMESTAMP WITH TIME ZONE,
ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}';

UPDATE simulation_runs
SET updated_at = COALESCE(updated_at, created_at, NOW()),
    status = COALESCE(NULLIF(status, ''), 'pending'),
    metadata = COALESCE(metadata, '{}'::jsonb);

CREATE INDEX IF NOT EXISTS idx_simulation_runs_user_created
ON simulation_runs(user_id, created_at DESC)
WHERE user_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_simulation_runs_project_created
ON simulation_runs(project_public_id, created_at DESC)
WHERE project_public_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_simulation_runs_engine_run_id
ON simulation_runs(engine_run_id)
WHERE engine_run_id IS NOT NULL;

DROP TRIGGER IF EXISTS update_simulation_runs_updated_at ON simulation_runs;
CREATE TRIGGER update_simulation_runs_updated_at
    BEFORE UPDATE ON simulation_runs
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

COMMENT ON COLUMN simulation_runs.user_id IS 'Firebase UID / application user that owns the simulation run';
COMMENT ON COLUMN simulation_runs.engine_run_id IS 'Simulation engine run ID mapped to the backend run_id';
COMMENT ON COLUMN simulation_runs.metadata IS 'Run metadata previously stored only in Redis';
