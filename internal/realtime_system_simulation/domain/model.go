package domain

import "time"

// SimulationRun represents a simulation run with its mapping
type SimulationRun struct {
	RunID           string    `json:"run_id"`
	UserID          string    `json:"user_id"`
	EngineRunID     string    `json:"engine_run_id"` // ID from the simulation engine
	Status          string    `json:"status"`        // pending, running, completed, failed, cancelled
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
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

