package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/domain"
	"github.com/google/uuid"
)

// SummaryRepository handles PostgreSQL operations for simulation summaries
type SummaryRepository struct {
	db *sql.DB
}

// NewSummaryRepository creates a new SummaryRepository
func NewSummaryRepository(db *sql.DB) *SummaryRepository {
	return &SummaryRepository{db: db}
}

// CreateOrUpdate creates or updates a simulation summary
// Uses ON CONFLICT to upsert based on run_id
func (r *SummaryRepository) CreateOrUpdate(summary *domain.SimulationSummary) error {
	// Generate UUID if not provided
	if summary.ID == "" {
		summary.ID = uuid.New().String()
	}

	query := `
		INSERT INTO simulation_summaries (
			id, run_id, engine_run_id, total_requests, total_errors, 
			total_duration_ms, metrics, summary_data
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (run_id) DO UPDATE SET
			engine_run_id = EXCLUDED.engine_run_id,
			total_requests = EXCLUDED.total_requests,
			total_errors = EXCLUDED.total_errors,
			total_duration_ms = EXCLUDED.total_duration_ms,
			metrics = EXCLUDED.metrics,
			summary_data = EXCLUDED.summary_data,
			updated_at = NOW()
		RETURNING created_at, updated_at
	`

	// Marshal JSONB fields
	metricsJSON, err := json.Marshal(summary.Metrics)
	if err != nil {
		metricsJSON = []byte("{}")
	}

	summaryDataJSON, err := json.Marshal(summary.SummaryData)
	if err != nil {
		summaryDataJSON = []byte("{}")
	}

	// Handle nullable fields
	var totalRequests, totalErrors, totalDuration sql.NullInt64
	if summary.TotalRequests > 0 {
		totalRequests = sql.NullInt64{Int64: summary.TotalRequests, Valid: true}
	}
	if summary.TotalErrors > 0 {
		totalErrors = sql.NullInt64{Int64: summary.TotalErrors, Valid: true}
	}
	if summary.TotalDuration > 0 {
		totalDuration = sql.NullInt64{Int64: summary.TotalDuration, Valid: true}
	}

	var createdAt, updatedAt time.Time
	err = r.db.QueryRow(
		query,
		summary.ID,
		summary.RunID,
		summary.EngineRunID,
		totalRequests,
		totalErrors,
		totalDuration,
		metricsJSON,
		summaryDataJSON,
	).Scan(&createdAt, &updatedAt)

	if err != nil {
		return fmt.Errorf("failed to create or update summary: %w", err)
	}

	summary.CreatedAt = createdAt
	summary.UpdatedAt = updatedAt

	return nil
}

// GetByRunID retrieves a summary by run ID
func (r *SummaryRepository) GetByRunID(runID string) (*domain.SimulationSummary, error) {
	query := `
		SELECT id, run_id, engine_run_id, total_requests, total_errors,
		       total_duration_ms, metrics, summary_data, created_at, updated_at
		FROM simulation_summaries
		WHERE run_id = $1
	`

	var summary domain.SimulationSummary
	var metricsJSON, summaryDataJSON []byte
	var totalRequests, totalErrors, totalDuration sql.NullInt64

	err := r.db.QueryRow(query, runID).Scan(
		&summary.ID,
		&summary.RunID,
		&summary.EngineRunID,
		&totalRequests,
		&totalErrors,
		&totalDuration,
		&metricsJSON,
		&summaryDataJSON,
		&summary.CreatedAt,
		&summary.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, domain.ErrRunNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get summary: %w", err)
	}

	// Handle nullable fields
	if totalRequests.Valid {
		summary.TotalRequests = totalRequests.Int64
	}
	if totalErrors.Valid {
		summary.TotalErrors = totalErrors.Int64
	}
	if totalDuration.Valid {
		summary.TotalDuration = totalDuration.Int64
	}

	// Unmarshal JSONB fields
	if len(metricsJSON) > 0 {
		if err := json.Unmarshal(metricsJSON, &summary.Metrics); err != nil {
			summary.Metrics = make(map[string]interface{})
		}
	} else {
		summary.Metrics = make(map[string]interface{})
	}

	if len(summaryDataJSON) > 0 {
		if err := json.Unmarshal(summaryDataJSON, &summary.SummaryData); err != nil {
			summary.SummaryData = make(map[string]interface{})
		}
	} else {
		summary.SummaryData = make(map[string]interface{})
	}

	return &summary, nil
}

// Exists checks if a summary exists for a given run ID
func (r *SummaryRepository) Exists(runID string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM simulation_summaries WHERE run_id = $1)`

	var exists bool
	err := r.db.QueryRow(query, runID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check if summary exists: %w", err)
	}

	return exists, nil
}
