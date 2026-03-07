-- Migration: Simulation Candidate S3 Path
-- Adds an optional S3 path column to simulation_candidates for per-candidate YAML storage.

ALTER TABLE simulation_candidates
ADD COLUMN IF NOT EXISTS s3_path TEXT;

