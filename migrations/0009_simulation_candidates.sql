-- Migration: Simulation Candidates
-- Stores parsed candidate configurations and metrics per simulation run.
--
-- This table is keyed by (user_id, project_public_id, run_id, candidate_id)
-- and is intended for querying candidate recommendations per run/project.

CREATE TABLE IF NOT EXISTS simulation_candidates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Ownership / scoping
    user_id TEXT NOT NULL,
    project_public_id TEXT,
    run_id VARCHAR(255) NOT NULL,

    -- Candidate identity (e.g. "c1", "m2")
    candidate_id TEXT NOT NULL,

    -- Candidate spec, metrics, and workload as JSONB for flexibility
    spec JSONB NOT NULL,
    metrics JSONB NOT NULL,
    sim_workload JSONB NOT NULL,

    -- Source of this candidate (e.g. simulation YAML path, engine run reference)
    source TEXT NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Foreign key to projects (optional; may be NULL if not associated)
ALTER TABLE simulation_candidates
ADD CONSTRAINT fk_simulation_candidates_project
FOREIGN KEY (project_public_id)
REFERENCES projects(public_id)
ON DELETE SET NULL;

-- Foreign key to simulation_runs reference table
ALTER TABLE simulation_candidates
ADD CONSTRAINT fk_simulation_candidates_run
FOREIGN KEY (run_id)
REFERENCES simulation_runs(run_id)
ON DELETE CASCADE;

-- Ensure one row per (run, candidate_id)
CREATE UNIQUE INDEX IF NOT EXISTS ux_simulation_candidates_run_candidate
ON simulation_candidates(run_id, candidate_id);

-- Indexes for common access patterns
CREATE INDEX IF NOT EXISTS idx_simulation_candidates_user
ON simulation_candidates(user_id);

CREATE INDEX IF NOT EXISTS idx_simulation_candidates_project
ON simulation_candidates(project_public_id)
WHERE project_public_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_simulation_candidates_run
ON simulation_candidates(run_id);

COMMENT ON TABLE simulation_candidates IS
  'Stores parsed candidate specs and metrics per simulation run (user, project, run, candidate_id).';

