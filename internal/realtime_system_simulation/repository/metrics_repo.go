package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// MetricsRepository handles persistence of simulation summaries and time-series metrics.
type MetricsRepository struct {
	db *sql.DB
}

// NewMetricsRepository creates a new MetricsRepository backed by the given DB.
func NewMetricsRepository(db *sql.DB) *MetricsRepository {
	return &MetricsRepository{db: db}
}

// SummaryRecord represents a row read from simulation_summaries.
type SummaryRecord struct {
	RunID          string
	EngineRunID    string
	Metrics        map[string]any
	SummaryData    map[string]any
	TotalRequests  sql.NullInt64
	TotalErrors    sql.NullInt64
	TotalDurationMs sql.NullInt64
}

// GetSummaryByRunID reads a summary row for the given run_id from simulation_summaries.
func (r *MetricsRepository) GetSummaryByRunID(ctx context.Context, runID string) (*SummaryRecord, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("metrics repository not properly initialized")
	}

	var rec SummaryRecord
	var metricsJSON, summaryJSON []byte

	err := r.db.QueryRowContext(ctx, `
        SELECT run_id, engine_run_id, metrics, summary_data,
               total_requests, total_errors, total_duration_ms
        FROM simulation_summaries
        WHERE run_id = $1
    `, runID).Scan(
		&rec.RunID,
		&rec.EngineRunID,
		&metricsJSON,
		&summaryJSON,
		&rec.TotalRequests,
		&rec.TotalErrors,
		&rec.TotalDurationMs,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query simulation_summaries for run_id=%s: %w", runID, err)
	}

	if err := json.Unmarshal(metricsJSON, &rec.Metrics); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metrics JSON for run_id=%s: %w", runID, err)
	}
	if err := json.Unmarshal(summaryJSON, &rec.SummaryData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal summary_data JSON for run_id=%s: %w", runID, err)
	}

	return &rec, nil
}

// ListTimeSeriesByRunID returns all timeseries points for the given run_id.
func (r *MetricsRepository) ListTimeSeriesByRunID(ctx context.Context, runID string) ([]TimeSeriesPoint, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("metrics repository not properly initialized")
	}

	rows, err := r.db.QueryContext(ctx, `
        SELECT run_id, time, timestamp_ms, metric_type, metric_value, service_id, node_id, tags
        FROM simulation_metrics_timeseries
        WHERE run_id = $1
        ORDER BY time ASC
    `, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to query simulation_metrics_timeseries for run_id=%s: %w", runID, err)
	}
	defer rows.Close()

	var out []TimeSeriesPoint
	for rows.Next() {
		var p TimeSeriesPoint
		var tagsJSON []byte
		if err := rows.Scan(
			&p.RunID,
			&p.Time,
			&p.TimestampMs,
			&p.MetricType,
			&p.MetricValue,
			&p.ServiceID,
			&p.NodeID,
			&tagsJSON,
		); err != nil {
			return nil, fmt.Errorf("failed to scan metrics_timeseries row for run_id=%s: %w", runID, err)
		}
		if len(tagsJSON) > 0 {
			if err := json.Unmarshal(tagsJSON, &p.Tags); err != nil {
				return nil, fmt.Errorf("failed to unmarshal tags JSON for run_id=%s metric=%s: %w", runID, p.MetricType, err)
			}
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating metrics_timeseries rows for run_id=%s: %w", runID, err)
	}

	return out, nil
}

// SummaryUpsertParams captures the data needed to upsert a simulation_summaries row.
type SummaryUpsertParams struct {
	RunID       string
	EngineRunID string
	Metrics     map[string]any
	SummaryData map[string]any
	ScenarioYAML string
}

// TimeSeriesPoint represents a single timeseries datapoint to persist.
type TimeSeriesPoint struct {
	RunID       string
	Time        time.Time
	TimestampMs int64
	MetricType  string
	MetricValue float64
	ServiceID   string
	NodeID      string
	Tags        map[string]any
}

// UpsertSummary upserts a row in simulation_summaries for the given run.
func (r *MetricsRepository) UpsertSummary(ctx context.Context, p *SummaryUpsertParams) error {
	if r == nil || r.db == nil || p == nil {
		return fmt.Errorf("metrics repository not properly initialized")
	}

	metricsJSON, err := json.Marshal(p.Metrics)
	if err != nil {
		return fmt.Errorf("failed to marshal metrics JSON: %w", err)
	}
	summaryJSON, err := json.Marshal(p.SummaryData)
	if err != nil {
		return fmt.Errorf("failed to marshal summary_data JSON: %w", err)
	}

	// Ensure there is a reference row in simulation_runs to satisfy FK; insert if missing.
	if _, err := r.db.ExecContext(
		ctx,
		`INSERT INTO simulation_runs (run_id) VALUES ($1)
         ON CONFLICT (run_id) DO NOTHING`,
		p.RunID,
	); err != nil {
		return fmt.Errorf("failed to ensure simulation_runs row for run_id=%s: %w", p.RunID, err)
	}

	_, err = r.db.ExecContext(
		ctx,
		`INSERT INTO simulation_summaries (run_id, engine_run_id, metrics, summary_data, scenario_yaml)
         VALUES ($1, $2, $3::jsonb, $4::jsonb, $5)
         ON CONFLICT (run_id) DO UPDATE
         SET engine_run_id = EXCLUDED.engine_run_id,
             metrics = EXCLUDED.metrics,
             summary_data = EXCLUDED.summary_data,
             scenario_yaml = COALESCE(EXCLUDED.scenario_yaml, simulation_summaries.scenario_yaml)`,
		p.RunID, p.EngineRunID, string(metricsJSON), string(summaryJSON), p.ScenarioYAML,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert simulation_summaries for run_id=%s: %w", p.RunID, err)
	}

	return nil
}

// InsertTimeSeries inserts a batch of timeseries points into simulation_metrics_timeseries.
func (r *MetricsRepository) InsertTimeSeries(ctx context.Context, points []TimeSeriesPoint) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("metrics repository not properly initialized")
	}
	if len(points) == 0 {
		return nil
	}

	// Ensure there is a reference row in simulation_runs to satisfy FK; insert if missing.
	// All points for a single call are expected to share the same RunID.
	runID := points[0].RunID
	if runID != "" {
		if _, err := r.db.ExecContext(
			ctx,
			`INSERT INTO simulation_runs (run_id) VALUES ($1)
             ON CONFLICT (run_id) DO NOTHING`,
			runID,
		); err != nil {
			return fmt.Errorf("failed to ensure simulation_runs row for run_id=%s before timeseries insert: %w", runID, err)
		}
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction for timeseries insert: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	stmt, err := tx.PrepareContext(ctx, `
        INSERT INTO simulation_metrics_timeseries
            (run_id, time, timestamp_ms, metric_type, metric_value, service_id, node_id, tags)
        VALUES
            ($1, $2, $3, $4, $5, $6, $7, $8::jsonb)
    `)
	if err != nil {
		return fmt.Errorf("failed to prepare timeseries insert statement: %w", err)
	}
	defer stmt.Close()

	for _, p := range points {
		tagsJSON, err := json.Marshal(p.Tags)
		if err != nil {
			return fmt.Errorf("failed to marshal tags JSON for run_id=%s metric=%s: %w", p.RunID, p.MetricType, err)
		}
		if _, err := stmt.ExecContext(
			ctx,
			p.RunID,
			p.Time,
			p.TimestampMs,
			p.MetricType,
			p.MetricValue,
			p.ServiceID,
			p.NodeID,
			string(tagsJSON),
		); err != nil {
			return fmt.Errorf("failed to insert timeseries point for run_id=%s metric=%s: %w", p.RunID, p.MetricType, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit timeseries insert transaction: %w", err)
	}

	return nil
}

