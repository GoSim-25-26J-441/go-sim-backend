# Timeseries Schema Efficiency Analysis

## Current Schema Overview

The current schema is designed for TimescaleDB compatibility but has some efficiency concerns for standard PostgreSQL.

## Efficiency Issues

### 1. **Composite Primary Key (id, time)**
```sql
PRIMARY KEY (id, time)
```
**Issue:** 
- In standard PostgreSQL, this creates a large index that includes both columns
- The `id` is a BIGSERIAL (auto-incrementing), so it's already unique
- Including `time` in PK adds overhead without much benefit for standard PostgreSQL

**Impact:** 
- Larger index size
- Slower inserts (index maintenance)
- The `id` alone would be sufficient as PK for standard PostgreSQL

**Recommendation:**
- For standard PostgreSQL: Use `id` as single-column PK
- For TimescaleDB: Keep composite PK (required for partitioning)

### 2. **Redundant Time Storage**
```sql
time TIMESTAMPTZ NOT NULL,
timestamp_ms BIGINT NOT NULL,
```
**Issue:**
- Storing both `time` and `timestamp_ms` duplicates data
- Both are indexed separately, doubling index overhead
- Application can convert between them easily

**Impact:**
- ~16 bytes per row wasted (8 bytes for timestamp_ms + index overhead)
- Two indexes instead of one for time-based queries

**Recommendation:**
- Keep `time` (TIMESTAMPTZ) as primary time column
- Remove `timestamp_ms` or make it a generated column:
  ```sql
  timestamp_ms BIGINT GENERATED ALWAYS AS (EXTRACT(EPOCH FROM time) * 1000) STORED
  ```

### 3. **Index Redundancy**
Current indexes:
```sql
-- Index on time
idx_metrics_timeseries_run_time (run_id, time DESC)
idx_metrics_timeseries_time (time DESC)

-- Index on timestamp_ms (redundant)
idx_metrics_timeseries_run_timestamp (run_id, timestamp_ms DESC)
idx_metrics_timeseries_timestamp (timestamp_ms DESC)
```

**Issue:**
- Maintaining 4 time-related indexes when 2 would suffice
- Doubles write overhead for time-based data

**Recommendation:**
- Remove `timestamp_ms` indexes if removing the column
- Or keep only `time`-based indexes

### 4. **Insert Performance**

**Current Approach:**
- Using prepared statements in transaction (good)
- Row-by-row inserts in loop (could be faster)

**Better Approach:**
- Use PostgreSQL `COPY` for bulk inserts (10-100x faster)
- Or use `INSERT ... VALUES (...), (...), (...)` with multiple rows

### 5. **VARCHAR for run_id**
```sql
run_id VARCHAR(255) NOT NULL
```
**Issue:**
- VARCHAR(255) is variable-length, adding overhead
- UUIDs are fixed 36 characters, but stored as VARCHAR

**Recommendation:**
- If run_id is UUID format, consider using UUID type
- Or use fixed CHAR(36) if always UUIDs

## Query Pattern Analysis

### Current Query Patterns:
1. `WHERE run_id = ? ORDER BY time ASC` - ✅ Well indexed
2. `WHERE run_id = ? AND metric_type = ?` - ✅ Well indexed  
3. `WHERE run_id = ? AND time BETWEEN ? AND ?` - ✅ Well indexed
4. `WHERE run_id = ? AND service_id = ?` - ✅ Well indexed

**Verdict:** Indexes match query patterns well.

## Recommendations by Scale

### Small Scale (< 1M rows per run)
**Current schema is acceptable** with minor optimizations:
- Remove redundant `timestamp_ms` column/indexes
- Use single-column PK: `PRIMARY KEY (id)`
- Consider COPY for bulk inserts

### Medium Scale (1M - 100M rows)
**Optimize for standard PostgreSQL:**
1. Single-column PK: `PRIMARY KEY (id)`
2. Remove `timestamp_ms` column (use generated column if needed)
3. Use COPY for bulk inserts
4. Consider partitioning by `run_id` or `time` range

### Large Scale (100M+ rows)
**Migrate to TimescaleDB:**
- Current schema is TimescaleDB-ready
- Hypertables provide automatic partitioning
- Compression for old data
- Continuous aggregates for pre-computed statistics

## Performance Improvements

### 1. Optimize Insert Performance
```go
// Current: Row-by-row prepared statement
// Better: Use COPY
func (r *MetricsTimeseriesRepository) InsertBatch(ctx context.Context, points []domain.MetricDataPoint) error {
    // Use COPY FROM for bulk insert (much faster)
    // Or batch INSERT with multiple VALUES
}
```

### 2. Remove Redundant Columns
```sql
-- Remove timestamp_ms, use generated column if needed
ALTER TABLE simulation_metrics_timeseries 
DROP COLUMN timestamp_ms;

-- Or make it generated
ALTER TABLE simulation_metrics_timeseries
ADD COLUMN timestamp_ms BIGINT GENERATED ALWAYS AS 
    (EXTRACT(EPOCH FROM time) * 1000) STORED;
```

### 3. Simplify Primary Key
```sql
-- For standard PostgreSQL
ALTER TABLE simulation_metrics_timeseries
DROP CONSTRAINT simulation_metrics_timeseries_pkey,
ADD PRIMARY KEY (id);

-- Keep composite PK only if migrating to TimescaleDB soon
```

## Migration Path

### Phase 1: Quick Wins (No Schema Change)
- Optimize insert to use COPY or batch VALUES
- Add connection pooling
- Monitor index usage, remove unused indexes

### Phase 2: Schema Optimization (Requires Migration)
- Remove `timestamp_ms` column
- Simplify primary key
- Remove redundant indexes

### Phase 3: Scale Up (When Needed)
- Migrate to TimescaleDB
- Enable compression
- Set up continuous aggregates

## Conclusion

**Current Efficiency Rating: 6/10**

**Strengths:**
- Good index coverage for query patterns
- TimescaleDB-ready design
- Proper use of transactions

**Weaknesses:**
- Redundant time storage
- Composite PK overhead for standard PostgreSQL
- Suboptimal bulk insert method
- Index redundancy

**Priority Fixes:**
1. **High:** Optimize bulk inserts (COPY or batch VALUES)
2. **Medium:** Remove `timestamp_ms` redundancy
3. **Low:** Simplify PK (only if not migrating to TimescaleDB soon)
