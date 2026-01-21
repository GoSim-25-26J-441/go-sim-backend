package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/domain"
)

// MetricsTimeseriesRepository handles PostgreSQL operations for timeseries metrics
type MetricsTimeseriesRepository struct {
	db *sql.DB
}

// NewMetricsTimeseriesRepository creates a new MetricsTimeseriesRepository
func NewMetricsTimeseriesRepository(db *sql.DB) *MetricsTimeseriesRepository {
	return &MetricsTimeseriesRepository{db: db}
}

// InsertBatch inserts multiple metric data points in a single transaction
// This is more efficient than inserting one at a time
func (r *MetricsTimeseriesRepository) InsertBatch(ctx context.Context, points []domain.MetricDataPoint) error {
	if len(points) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO simulation_metrics_timeseries (
			run_id, time, timestamp_ms, metric_type, metric_value,
			service_id, node_id, tags
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, point := range points {
		// Ensure both time and timestamp_ms are set
		if point.Time.IsZero() && point.TimestampMs > 0 {
			point.Time = time.Unix(0, point.TimestampMs*int64(time.Millisecond))
		} else if point.TimestampMs == 0 && !point.Time.IsZero() {
			point.TimestampMs = point.Time.UnixMilli()
		} else if point.Time.IsZero() && point.TimestampMs == 0 {
			point.Time = time.Now()
			point.TimestampMs = point.Time.UnixMilli()
		}

		// Marshal tags JSONB
		tagsJSON, err := json.Marshal(point.Tags)
		if err != nil {
			tagsJSON = []byte("{}")
		}

		_, err = stmt.ExecContext(ctx,
			point.RunID,
			point.Time,
			point.TimestampMs,
			point.MetricType,
			point.MetricValue,
			point.ServiceID,
			point.NodeID,
			tagsJSON,
		)
		if err != nil {
			return fmt.Errorf("failed to insert metric point: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetByRunID retrieves all metrics for a given run ID
func (r *MetricsTimeseriesRepository) GetByRunID(ctx context.Context, runID string) ([]domain.MetricDataPoint, error) {
	return r.GetByRunIDAndType(ctx, runID, "")
}

// GetByRunIDAndType retrieves metrics for a run ID, optionally filtered by metric type
func (r *MetricsTimeseriesRepository) GetByRunIDAndType(ctx context.Context, runID string, metricType string) ([]domain.MetricDataPoint, error) {
	var query string
	var args []interface{}

	if metricType != "" {
		query = `
			SELECT id, run_id, time, timestamp_ms, metric_type, metric_value,
			       service_id, node_id, tags
			FROM simulation_metrics_timeseries
			WHERE run_id = $1 AND metric_type = $2
			ORDER BY time ASC
		`
		args = []interface{}{runID, metricType}
	} else {
		query = `
			SELECT id, run_id, time, timestamp_ms, metric_type, metric_value,
			       service_id, node_id, tags
			FROM simulation_metrics_timeseries
			WHERE run_id = $1
			ORDER BY time ASC
		`
		args = []interface{}{runID}
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query metrics: %w", err)
	}
	defer rows.Close()

	var points []domain.MetricDataPoint
	for rows.Next() {
		var point domain.MetricDataPoint
		var tagsJSON []byte
		var serviceID, nodeID sql.NullString

		err := rows.Scan(
			&point.ID,
			&point.RunID,
			&point.Time,
			&point.TimestampMs,
			&point.MetricType,
			&point.MetricValue,
			&serviceID,
			&nodeID,
			&tagsJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan metric point: %w", err)
		}

		// Handle nullable fields
		if serviceID.Valid {
			point.ServiceID = &serviceID.String
		}
		if nodeID.Valid {
			point.NodeID = &nodeID.String
		}

		// Unmarshal tags JSONB
		if len(tagsJSON) > 0 {
			if err := json.Unmarshal(tagsJSON, &point.Tags); err != nil {
				point.Tags = make(map[string]interface{})
			}
		} else {
			point.Tags = make(map[string]interface{})
		}

		points = append(points, point)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating metrics: %w", err)
	}

	return points, nil
}

// GetByRunIDAndTimeRange retrieves metrics for a run ID within a time range
func (r *MetricsTimeseriesRepository) GetByRunIDAndTimeRange(
	ctx context.Context,
	runID string,
	fromTime *time.Time,
	toTime *time.Time,
	metricType string,
) ([]domain.MetricDataPoint, error) {
	query := `
		SELECT id, run_id, time, timestamp_ms, metric_type, metric_value,
		       service_id, node_id, tags
		FROM simulation_metrics_timeseries
		WHERE run_id = $1
	`
	args := []interface{}{runID}
	argIndex := 2

	// Add time range filters
	if fromTime != nil {
		query += fmt.Sprintf(" AND time >= $%d", argIndex)
		args = append(args, *fromTime)
		argIndex++
	}
	if toTime != nil {
		query += fmt.Sprintf(" AND time <= $%d", argIndex)
		args = append(args, *toTime)
		argIndex++
	}

	// Add metric type filter
	if metricType != "" {
		query += fmt.Sprintf(" AND metric_type = $%d", argIndex)
		args = append(args, metricType)
		argIndex++
	}

	query += " ORDER BY time ASC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query metrics: %w", err)
	}
	defer rows.Close()

	var points []domain.MetricDataPoint
	for rows.Next() {
		var point domain.MetricDataPoint
		var tagsJSON []byte
		var serviceID, nodeID sql.NullString

		err := rows.Scan(
			&point.ID,
			&point.RunID,
			&point.Time,
			&point.TimestampMs,
			&point.MetricType,
			&point.MetricValue,
			&serviceID,
			&nodeID,
			&tagsJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan metric point: %w", err)
		}

		// Handle nullable fields
		if serviceID.Valid {
			point.ServiceID = &serviceID.String
		}
		if nodeID.Valid {
			point.NodeID = &nodeID.String
		}

		// Unmarshal tags JSONB
		if len(tagsJSON) > 0 {
			if err := json.Unmarshal(tagsJSON, &point.Tags); err != nil {
				point.Tags = make(map[string]interface{})
			}
		} else {
			point.Tags = make(map[string]interface{})
		}

		points = append(points, point)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating metrics: %w", err)
	}

	return points, nil
}

// CountByRunID returns the number of metric points for a given run ID
func (r *MetricsTimeseriesRepository) CountByRunID(ctx context.Context, runID string) (int64, error) {
	query := `SELECT COUNT(*) FROM simulation_metrics_timeseries WHERE run_id = $1`

	var count int64
	err := r.db.QueryRowContext(ctx, query, runID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count metrics: %w", err)
	}

	return count, nil
}
