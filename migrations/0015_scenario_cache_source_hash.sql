-- Track AMG/APD source content hash for cache invalidation and generation vs edit flows.
ALTER TABLE simulation_scenario_cache
ADD COLUMN IF NOT EXISTS source_hash TEXT;

COMMENT ON COLUMN simulation_scenario_cache.source_hash IS
  'SHA-256 hex of the AMG/APD diagram YAML used when scenario was generated; empty for request-only cache rows.';
