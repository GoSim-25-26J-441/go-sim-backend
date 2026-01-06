package service

import (
	"time"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/repository"
)

// SimulationService handles business logic for simulation runs
type SimulationService struct {
	runRepo *repository.RunRepository
}

// NewSimulationService creates a new SimulationService
func NewSimulationService(runRepo *repository.RunRepository) *SimulationService {
	return &SimulationService{
		runRepo: runRepo,
	}
}

// CreateRun creates a new simulation run
func (s *SimulationService) CreateRun(req *domain.CreateRunRequest) (*domain.SimulationRun, error) {
	run := &domain.SimulationRun{
		UserID:    req.UserID,
		Status:    domain.StatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Metadata:  req.Metadata,
	}

	if run.Metadata == nil {
		run.Metadata = make(map[string]interface{})
	}

	if err := s.runRepo.Create(run); err != nil {
		return nil, err
	}

	return run, nil
}

// GetRun retrieves a run by its ID
func (s *SimulationService) GetRun(runID string) (*domain.SimulationRun, error) {
	return s.runRepo.GetByRunID(runID)
}

// GetRunByEngineID retrieves a run by the engine's run ID
func (s *SimulationService) GetRunByEngineID(engineRunID string) (*domain.SimulationRun, error) {
	return s.runRepo.GetByEngineRunID(engineRunID)
}

// UpdateRun updates an existing run
func (s *SimulationService) UpdateRun(runID string, req *domain.UpdateRunRequest) (*domain.SimulationRun, error) {
	run, err := s.runRepo.GetByRunID(runID)
	if err != nil {
		return nil, err
	}

	// Update fields if provided
	if req.Status != nil {
		if !isValidStatus(*req.Status) {
			return nil, domain.ErrInvalidStatus
		}
		run.Status = *req.Status

		// Set completed_at if status is completed or failed
		if *req.Status == domain.StatusCompleted || *req.Status == domain.StatusFailed {
			now := time.Now()
			run.CompletedAt = &now
		}
	}

	if req.EngineRunID != nil {
		run.EngineRunID = *req.EngineRunID
	}

	// Merge metadata if provided
	if req.Metadata != nil && len(req.Metadata) > 0 {
		if run.Metadata == nil {
			run.Metadata = make(map[string]interface{})
		}
		for k, v := range req.Metadata {
			run.Metadata[k] = v
		}
	}

	if err := s.runRepo.Update(run); err != nil {
		return nil, err
	}

	return run, nil
}

// ListRunsByUser retrieves all run IDs for a user
func (s *SimulationService) ListRunsByUser(userID string) ([]string, error) {
	return s.runRepo.ListByUserID(userID)
}

// DeleteRun deletes a run
func (s *SimulationService) DeleteRun(runID string) error {
	return s.runRepo.Delete(runID)
}

// isValidStatus checks if a status is valid
func isValidStatus(status string) bool {
	return status == domain.StatusPending ||
		status == domain.StatusRunning ||
		status == domain.StatusCompleted ||
		status == domain.StatusFailed ||
		status == domain.StatusCancelled
}
