ALTER TABLE simulation_summaries
ADD COLUMN IF NOT EXISTS final_config JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN simulation_summaries.final_config IS
  'Final run configuration exported by simulation-core (e.g., final_config.placements).';
