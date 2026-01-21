package domain

import "time"

// SimulationRun represents a simulation run with its mapping
type SimulationRun struct {
	RunID       string                 `json:"run_id"`
	UserID      string                 `json:"user_id"`
	EngineRunID string                 `json:"engine_run_id"` // ID from the simulation engine
	Status      string                 `json:"status"`        // pending, running, completed, failed, cancelled
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// RunStatus constants
const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
)

// CreateRunRequest represents data needed to create a new simulation run
type CreateRunRequest struct {
	UserID   string
	Metadata map[string]interface{}
}

// UpdateRunRequest represents data for updating a simulation run
type UpdateRunRequest struct {
	Status      *string
	EngineRunID *string
	Metadata    map[string]interface{}
}

// SimulationSummary represents the persisted summary for a completed simulation run.
// This maps to the simulation_summaries table defined in migrations/0002_simulation_data_storage.sql.
type SimulationSummary struct {
	ID            string `json:"id"`            // UUID (from DB)
	RunID         string `json:"run_id"`        // FK to SimulationRun.RunID
	EngineRunID   string `json:"engine_run_id"` // ID in simulation engine
	TotalRequests int64  `json:"total_requests"`
	TotalErrors   int64  `json:"total_errors"`
	TotalDuration int64  `json:"total_duration_ms"`
	// Aggregated metrics and additional summary data are stored as flexible JSON blobs.
	Metrics     map[string]interface{} `json:"metrics,omitempty"`      // JSONB metrics column
	SummaryData map[string]interface{} `json:"summary_data,omitempty"` // JSONB summary_data column
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// MetricDataPoint represents a single timeseries metric row for a simulation run.
// This maps to the simulation_metrics_timeseries table.
type MetricDataPoint struct {
	ID          int64                  `json:"id"`             // BIGSERIAL primary key
	RunID       string                 `json:"run_id"`         // FK to SimulationRun.RunID
	Time        time.Time              `json:"time"`           // TIMESTAMPTZ (primary time column)
	TimestampMs int64                  `json:"timestamp_ms"`   // Unix timestamp in ms (duplicate for convenience)
	MetricType  string                 `json:"metric_type"`    // e.g. request_latency_ms, cpu_utilization
	MetricValue float64                `json:"metric_value"`   // numeric value
	ServiceID   *string                `json:"service_id"`     // optional
	NodeID      *string                `json:"node_id"`        // optional
	Tags        map[string]interface{} `json:"tags,omitempty"` // JSONB tags column
}
