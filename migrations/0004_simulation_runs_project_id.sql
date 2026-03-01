-- Migration: Associate simulation runs with projects
-- Adds project_public_id to simulation_runs for project-scoped run management
--
-- Prerequisites: 0001 (users), 0002 (projects), 0003 (simulation_runs)

-- Add project_public_id column (nullable for backward compatibility with existing runs)
ALTER TABLE simulation_runs
ADD COLUMN IF NOT EXISTS project_public_id TEXT;

-- Foreign key to projects
ALTER TABLE simulation_runs
DROP CONSTRAINT IF EXISTS fk_simulation_runs_project;

ALTER TABLE simulation_runs
ADD CONSTRAINT fk_simulation_runs_project
FOREIGN KEY (project_public_id)
REFERENCES projects(public_id)
ON DELETE SET NULL;

-- Index for project-scoped queries
CREATE INDEX IF NOT EXISTS idx_simulation_runs_project
ON simulation_runs(project_public_id)
WHERE project_public_id IS NOT NULL;

COMMENT ON COLUMN simulation_runs.project_public_id IS 'Optional: associates the run with a project (projects.public_id)';
