-- Migration: Simulation Data Storage
-- Stores simulation summaries and timeseries metrics data
--
-- DESIGN: This schema is designed to work with standard PostgreSQL now,
-- but is TimescaleDB-ready for easy migration in the future.
--
-- Key design decisions for future TimescaleDB migration:
-- 1. Uses TIMESTAMPTZ column ('time') which is required for TimescaleDB
-- 2. Primary key includes time column (recommended for TimescaleDB)
-- 3. Maintains both 'time' and 'timestamp_ms' for compatibility
-- 4. Indexes are optimized for both PostgreSQL and TimescaleDB
--
-- Note: This migration assumes simulation_runs table exists in PostgreSQL.
-- If runs are only stored in Redis, you may want to:
--   1. Create a minimal simulation_runs table here for foreign key reference
--   2. Or remove the foreign key constraints and use run_id as plain VARCHAR

-- Create minimal simulation_runs table if it doesn't exist (for foreign key reference)
-- This is just a reference table - actual run data may be in Redis
CREATE TABLE IF NOT EXISTS simulation_runs (
    run_id VARCHAR(255) PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Simulation Summaries Table
-- Stores aggregated statistics and summary data for completed simulations
CREATE TABLE IF NOT EXISTS simulation_summaries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id VARCHAR(255) NOT NULL UNIQUE,
    engine_run_id VARCHAR(255) NOT NULL,
    
    -- Overall statistics
    total_requests BIGINT,
    total_errors BIGINT,
    total_duration_ms BIGINT,
    
    -- Aggregated metrics stored as JSONB for flexibility
    -- Structure: {"metric_name": {"avg": value, "p50": value, "p95": value, "max": value, "min": value}, ...}
    -- Example metrics: request_latency, cpu_utilization, memory_utilization, throughput_rps
    metrics JSONB NOT NULL DEFAULT '{}',
    
    -- Additional summary data
    summary_data JSONB DEFAULT '{}',
    
    -- Metadata
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- Foreign key constraint
    CONSTRAINT fk_simulation_summaries_run_id 
        FOREIGN KEY (run_id) 
        REFERENCES simulation_runs(run_id) 
        ON DELETE CASCADE
);

-- Simulation Metrics Timeseries Table
-- Stores point-in-time metrics collected during simulation execution
-- 
-- DESIGN NOTE: This schema is designed for easy migration to TimescaleDB in the future.
-- The 'time' column uses TIMESTAMPTZ which is required for TimescaleDB hypertables.
-- When ready to migrate to TimescaleDB, simply run:
--   CREATE EXTENSION IF NOT EXISTS timescaledb;
--   SELECT create_hypertable('simulation_metrics_timeseries', 'time');
--
CREATE TABLE IF NOT EXISTS simulation_metrics_timeseries (
    id BIGSERIAL,
    run_id VARCHAR(255) NOT NULL,
    
    -- Primary timestamp column (TIMESTAMPTZ for TimescaleDB compatibility)
    -- This allows easy conversion to TimescaleDB hypertable
    time TIMESTAMPTZ NOT NULL,
    
    -- Also store timestamp_ms for application compatibility and queries
    -- This can be indexed separately for performance
    timestamp_ms BIGINT NOT NULL,
    
    -- Metric identification
    metric_type VARCHAR(100) NOT NULL, 
    -- Examples: 'request_latency_ms', 'cpu_utilization', 'memory_utilization', 
    --          'request_count', 'request_error_count', 'throughput_rps', 'queue_length'
    
    -- Metric value
    metric_value DOUBLE PRECISION NOT NULL,
    
    -- Optional context
    service_id VARCHAR(255), -- Which service/node this metric belongs to
    node_id VARCHAR(255),    -- Specific node identifier
    tags JSONB DEFAULT '{}', -- Additional context as key-value pairs
    
    -- Primary key includes time for TimescaleDB partitioning compatibility
    -- Note: Including 'time' in PK is recommended for TimescaleDB
    PRIMARY KEY (id, time),
    
    -- Foreign key constraint
    CONSTRAINT fk_metrics_timeseries_run_id 
        FOREIGN KEY (run_id) 
        REFERENCES simulation_runs(run_id) 
        ON DELETE CASCADE
);

-- Indexes for efficient querying
-- These indexes work well with both standard PostgreSQL and TimescaleDB

-- Primary lookup: get all metrics for a run ordered by time
-- Using 'time' column (TIMESTAMPTZ) for better TimescaleDB compatibility
CREATE INDEX IF NOT EXISTS idx_metrics_timeseries_run_time 
    ON simulation_metrics_timeseries(run_id, time DESC);

-- Also index timestamp_ms for application compatibility
CREATE INDEX IF NOT EXISTS idx_metrics_timeseries_run_timestamp 
    ON simulation_metrics_timeseries(run_id, timestamp_ms DESC);

-- Get specific metric type for a run
CREATE INDEX IF NOT EXISTS idx_metrics_timeseries_run_type 
    ON simulation_metrics_timeseries(run_id, metric_type);

-- Time-based queries (useful for cross-run analysis)
-- Using TIMESTAMPTZ for TimescaleDB optimization
CREATE INDEX IF NOT EXISTS idx_metrics_timeseries_time 
    ON simulation_metrics_timeseries(time DESC);

-- Also keep timestamp_ms index for backward compatibility
CREATE INDEX IF NOT EXISTS idx_metrics_timeseries_timestamp 
    ON simulation_metrics_timeseries(timestamp_ms DESC);

-- Composite index for service-specific queries
CREATE INDEX IF NOT EXISTS idx_metrics_timeseries_run_service 
    ON simulation_metrics_timeseries(run_id, service_id, time DESC);

-- Index for summary lookup
CREATE INDEX IF NOT EXISTS idx_simulation_summaries_run_id 
    ON simulation_summaries(run_id);

-- Index for JSONB metrics queries (PostgreSQL GIN index)
CREATE INDEX IF NOT EXISTS idx_simulation_summaries_metrics 
    ON simulation_summaries USING GIN (metrics);

-- Update timestamp trigger function (reuse existing if available)
-- Create if it doesn't exist (from 0001_auth_users.sql)
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Apply trigger to simulation_summaries
DROP TRIGGER IF EXISTS update_simulation_summaries_updated_at ON simulation_summaries;
CREATE TRIGGER update_simulation_summaries_updated_at 
    BEFORE UPDATE ON simulation_summaries
    FOR EACH ROW 
    EXECUTE FUNCTION update_updated_at_column();

-- Comments for documentation
COMMENT ON TABLE simulation_summaries IS 'Stores aggregated summary statistics for completed simulation runs';
COMMENT ON TABLE simulation_metrics_timeseries IS 'Stores timeseries metrics data collected during simulation execution';

COMMENT ON COLUMN simulation_summaries.metrics IS 'JSONB object containing aggregated metric statistics. Format: {"metric_name": {"avg": value, "p50": value, "p95": value, "max": value, "min": value}}';

COMMENT ON TABLE simulation_metrics_timeseries IS 'Timeseries metrics data. Designed for easy migration to TimescaleDB - the ''time'' column uses TIMESTAMPTZ required for hypertables.';
COMMENT ON COLUMN simulation_metrics_timeseries.time IS 'Timestamp with timezone - primary time column for TimescaleDB compatibility. Use this for time-based queries and TimescaleDB migration.';
COMMENT ON COLUMN simulation_metrics_timeseries.timestamp_ms IS 'Unix timestamp in milliseconds - stored for application compatibility. Automatically maintained in sync with ''time'' column.';
COMMENT ON COLUMN simulation_metrics_timeseries.metric_type IS 'Type of metric: request_latency_ms, cpu_utilization, memory_utilization, request_count, etc.';
COMMENT ON COLUMN simulation_metrics_timeseries.tags IS 'Additional context as JSONB key-value pairs (e.g., {"host": "gateway-1", "region": "us-east"})';
