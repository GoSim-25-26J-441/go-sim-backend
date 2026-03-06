package domain

import "time"

// SimulationRun represents a simulation run with its mapping
type SimulationRun struct {
	RunID           string                 `json:"run_id"`
	UserID          string                 `json:"user_id"`
	ProjectPublicID string                 `json:"project_id,omitempty"` // Optional: associates run with a project
	EngineRunID     string                 `json:"engine_run_id"`        // ID from the simulation engine
	Status          string                 `json:"status"`               // pending, running, completed, failed, cancelled, stopped
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
	CompletedAt     *time.Time             `json:"completed_at,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// RunStatus constants
const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
	StatusStopped   = "stopped"
)

// CreateRunRequest represents data needed to create a new simulation run
type CreateRunRequest struct {
	UserID          string
	ProjectPublicID string                 // Optional: associates run with a project
	Metadata        map[string]interface{}
}

// UpdateRunRequest represents data for updating a simulation run
type UpdateRunRequest struct {
	Status      *string
	EngineRunID *string
	Metadata    map[string]interface{}
}

