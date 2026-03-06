-- Migration: Simulation Scenario YAML
-- Adds a column to store the scenario.yaml content for a simulation run in simulation_summaries.
--
-- This complements the existing best_candidate_s3_path, so that the backend can
-- retrieve the scenario directly from Postgres in addition to S3.

ALTER TABLE simulation_summaries
ADD COLUMN IF NOT EXISTS scenario_yaml TEXT;

