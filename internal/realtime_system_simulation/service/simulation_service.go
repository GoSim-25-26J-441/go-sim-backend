package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/repository"
)

// SimulationService handles business logic for simulation runs
type SimulationService struct {
	runRepo       *repository.RunRepository
	summaryRepo   *repository.SummaryRepository
	metricsRepo   *repository.MetricsTimeseriesRepository
	engineClient  EngineClient // Interface for fetching data from simulation engine
}

// EngineClient interface for fetching data from simulation engine
// This allows for easier testing and abstraction
type EngineClient interface {
	GetRunSummary(engineRunID string) (*RunSummaryResponse, error)
	GetRunMetrics(engineRunID string) ([]MetricDataPointResponse, error)
}

// RunSummaryResponse represents summary data from engine
// Matches the structure from http.RunSummaryResponse
type RunSummaryResponse struct {
	Summary struct {
		RunID            string                 `json:"run_id"`
		TotalRequests    int64                  `json:"total_requests,omitempty"`
		TotalErrors      int64                  `json:"total_errors,omitempty"`
		TotalDurationMs  int64                  `json:"total_duration_ms,omitempty"`
		Metrics          map[string]interface{} `json:"metrics,omitempty"`
		SummaryData      map[string]interface{} `json:"summary_data,omitempty"`
		CreatedAtUnixMs  int64                  `json:"created_at_unix_ms,omitempty"`
		StartedAtUnixMs  int64                  `json:"started_at_unix_ms,omitempty"`
		EndedAtUnixMs    int64                  `json:"ended_at_unix_ms,omitempty"`
	} `json:"summary"`
}

// MetricDataPointResponse represents a metric point from engine
// Matches the structure from http.MetricDataPointResponse
type MetricDataPointResponse struct {
	TimestampMs int64                  `json:"timestamp_ms"`
	MetricType  string                 `json:"metric_type"`
	MetricValue float64                `json:"metric_value"`
	ServiceID   string                 `json:"service_id,omitempty"`
	NodeID      string                 `json:"node_id,omitempty"`
	Tags        map[string]interface{} `json:"tags,omitempty"`
}

// NewSimulationService creates a new SimulationService
func NewSimulationService(runRepo *repository.RunRepository) *SimulationService {
	return &SimulationService{
		runRepo: runRepo,
	}
}

// NewSimulationServiceWithPersistence creates a new SimulationService with persistence support
func NewSimulationServiceWithPersistence(
	runRepo *repository.RunRepository,
	summaryRepo *repository.SummaryRepository,
	metricsRepo *repository.MetricsTimeseriesRepository,
	engineClient EngineClient,
) *SimulationService {
	return &SimulationService{
		runRepo:      runRepo,
		summaryRepo:  summaryRepo,
		metricsRepo:  metricsRepo,
		engineClient: engineClient,
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

// StoreRunSummaryAndMetrics fetches summary and metrics from the simulation engine
// and persists them to PostgreSQL. This should be called when a run completes.
func (s *SimulationService) StoreRunSummaryAndMetrics(ctx context.Context, runID string) error {
	if s.summaryRepo == nil || s.metricsRepo == nil || s.engineClient == nil {
		return fmt.Errorf("persistence repositories or engine client not initialized")
	}

	// Get the run to find engine run ID
	run, err := s.runRepo.GetByRunID(runID)
	if err != nil {
		return fmt.Errorf("failed to get run: %w", err)
	}

	if run.EngineRunID == "" {
		return fmt.Errorf("run %s has no engine run ID", runID)
	}

	// Fetch summary from engine
	summaryResp, err := s.engineClient.GetRunSummary(run.EngineRunID)
	if err != nil {
		log.Printf("Failed to fetch summary for run_id=%s, engine_run_id=%s: %v", runID, run.EngineRunID, err)
		// Continue to try metrics even if summary fails
	} else {
		// Map engine response to domain model
		summary := &domain.SimulationSummary{
			RunID:         runID,
			EngineRunID:   summaryResp.Summary.RunID,
			TotalRequests: summaryResp.Summary.TotalRequests,
			TotalErrors:   summaryResp.Summary.TotalErrors,
			TotalDuration: summaryResp.Summary.TotalDurationMs,
			Metrics:       summaryResp.Summary.Metrics,
			SummaryData:   summaryResp.Summary.SummaryData,
		}

		// Persist summary
		if err := s.summaryRepo.CreateOrUpdate(summary); err != nil {
			log.Printf("Failed to persist summary for run_id=%s: %v", runID, err)
			// Continue to try metrics
		} else {
			log.Printf("Successfully persisted summary for run_id=%s", runID)
		}
	}

	// Fetch metrics from engine
	metricsResp, err := s.engineClient.GetRunMetrics(run.EngineRunID)
	if err != nil {
		return fmt.Errorf("failed to fetch metrics for run_id=%s, engine_run_id=%s: %w", runID, run.EngineRunID, err)
	}

	if len(metricsResp) == 0 {
		log.Printf("No metrics returned for run_id=%s", runID)
		return nil
	}

	// Map engine responses to domain models
	metricPoints := make([]domain.MetricDataPoint, 0, len(metricsResp))
	for _, resp := range metricsResp {
		point := domain.MetricDataPoint{
			RunID:       runID,
			TimestampMs: resp.TimestampMs,
			Time:        time.Unix(0, resp.TimestampMs*int64(time.Millisecond)),
			MetricType:  resp.MetricType,
			MetricValue: resp.MetricValue,
			Tags:        resp.Tags,
		}

		if resp.ServiceID != "" {
			point.ServiceID = &resp.ServiceID
		}
		if resp.NodeID != "" {
			point.NodeID = &resp.NodeID
		}

		metricPoints = append(metricPoints, point)
	}

	// Persist metrics in batch
	if err := s.metricsRepo.InsertBatch(ctx, metricPoints); err != nil {
		return fmt.Errorf("failed to persist metrics for run_id=%s: %w", runID, err)
	}

	log.Printf("Successfully persisted %d metric points for run_id=%s", len(metricPoints), runID)
	return nil
}

// GetStoredSummary retrieves a persisted summary by run ID
func (s *SimulationService) GetStoredSummary(runID string) (*domain.SimulationSummary, error) {
	if s.summaryRepo == nil {
		return nil, fmt.Errorf("summary repository not initialized")
	}
	return s.summaryRepo.GetByRunID(runID)
}

// GetStoredMetrics retrieves persisted metrics by run ID
func (s *SimulationService) GetStoredMetrics(ctx context.Context, runID string, metricType string) ([]domain.MetricDataPoint, error) {
	if s.metricsRepo == nil {
		return nil, fmt.Errorf("metrics repository not initialized")
	}
	return s.metricsRepo.GetByRunIDAndType(ctx, runID, metricType)
}

// GetStoredMetricsWithTimeRange retrieves persisted metrics by run ID within a time range
func (s *SimulationService) GetStoredMetricsWithTimeRange(
	ctx context.Context,
	runID string,
	fromTime *time.Time,
	toTime *time.Time,
	metricType string,
) ([]domain.MetricDataPoint, error) {
	if s.metricsRepo == nil {
		return nil, fmt.Errorf("metrics repository not initialized")
	}
	return s.metricsRepo.GetByRunIDAndTimeRange(ctx, runID, fromTime, toTime, metricType)
}

// isValidStatus checks if a status is valid
func isValidStatus(status string) bool {
	return status == domain.StatusPending ||
		status == domain.StatusRunning ||
		status == domain.StatusCompleted ||
		status == domain.StatusFailed ||
		status == domain.StatusCancelled
}
