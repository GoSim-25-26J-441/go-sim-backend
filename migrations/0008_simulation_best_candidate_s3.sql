-- Migration: Simulation Best Candidate S3 Path
-- Adds a column to store the S3 path for the best candidate scenario per run.
-- The actual YAML is stored in S3 under a convention like:
--   s3://{S3_BUCKET}/simulation/{run_id}/best_scenario.yaml

-- Add best_candidate_s3_path column to simulation_summaries (one row per run)
ALTER TABLE simulation_summaries
ADD COLUMN IF NOT EXISTS best_candidate_s3_path TEXT;

COMMENT ON COLUMN simulation_summaries.best_candidate_s3_path IS
  'S3 object key or URL pointing to the best candidate scenario YAML (e.g. simulation/{run_id}/best_scenario.yaml)';

