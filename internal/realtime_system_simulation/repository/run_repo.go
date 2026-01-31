package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/domain"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	runKeyPrefix          = "sim:run:"         // Key prefix for run data: sim:run:{run_id}
	userRunSetPrefix      = "sim:user:"        // Set of run IDs for a user: sim:user:{user_id}
	engineRunIDPrefix     = "sim:engine:"      // Mapping from engine run ID to our run ID: sim:engine:{engine_run_id} -> run_id
	runEventChannelPrefix = "sim:events:"      // Pub/Sub channel for run events: sim:events:{run_id}
	runTTL                = 7 * 24 * time.Hour // TTL for run data (7 days)
)

// RunRepository handles Redis operations for simulation runs
type RunRepository struct {
	client *redis.Client
	ctx    context.Context
}

// NewRunRepository creates a new RunRepository
func NewRunRepository(client *redis.Client) *RunRepository {
	return &RunRepository{
		client: client,
		ctx:    context.Background(),
	}
}

// Create creates a new simulation run
func (r *RunRepository) Create(run *domain.SimulationRun) error {
	if run.RunID == "" {
		run.RunID = uuid.New().String()
	}
	if run.CreatedAt.IsZero() {
		run.CreatedAt = time.Now()
	}
	if run.UpdatedAt.IsZero() {
		run.UpdatedAt = time.Now()
	}

	runKey := r.runKey(run.RunID)
	userRunSetKey := r.userRunSetKey(run.UserID)

	// Serialize run data
	runData, err := json.Marshal(run)
	if err != nil {
		return fmt.Errorf("failed to marshal run data: %w", err)
	}

	// Use pipeline for atomic operations
	pipe := r.client.Pipeline()
	pipe.Set(r.ctx, runKey, runData, runTTL)
	pipe.SAdd(r.ctx, userRunSetKey, run.RunID)
	pipe.Expire(r.ctx, userRunSetKey, runTTL)

	// If engine run ID is provided, create index mapping
	if run.EngineRunID != "" {
		engineKey := r.engineRunIDKey(run.EngineRunID)
		pipe.Set(r.ctx, engineKey, run.RunID, runTTL)
	}

	_, err = pipe.Exec(r.ctx)
	if err != nil {
		return fmt.Errorf("failed to create run: %w", err)
	}

	return nil
}

// GetByRunID retrieves a run by its ID
func (r *RunRepository) GetByRunID(runID string) (*domain.SimulationRun, error) {
	runKey := r.runKey(runID)

	data, err := r.client.Get(r.ctx, runKey).Result()
	if err == redis.Nil {
		return nil, domain.ErrRunNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get run: %w", err)
	}

	var run domain.SimulationRun
	if err := json.Unmarshal([]byte(data), &run); err != nil {
		return nil, fmt.Errorf("failed to unmarshal run data: %w", err)
	}

	return &run, nil
}

// GetByEngineRunID retrieves a run by the engine's run ID
func (r *RunRepository) GetByEngineRunID(engineRunID string) (*domain.SimulationRun, error) {
	engineKey := r.engineRunIDKey(engineRunID)

	// Get our run ID from the engine run ID index
	runID, err := r.client.Get(r.ctx, engineKey).Result()
	if err == redis.Nil {
		return nil, domain.ErrRunNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get run ID from engine run ID: %w", err)
	}

	// Get the run using our run ID
	return r.GetByRunID(runID)
}

// Update updates an existing run
func (r *RunRepository) Update(run *domain.SimulationRun) error {
	run.UpdatedAt = time.Now()

	// Get existing run to check if engine run ID changed
	existing, err := r.GetByRunID(run.RunID)
	if err != nil {
		return err
	}

	runKey := r.runKey(run.RunID)

	runData, err := json.Marshal(run)
	if err != nil {
		return fmt.Errorf("failed to marshal run data: %w", err)
	}

	pipe := r.client.Pipeline()
	pipe.Set(r.ctx, runKey, runData, runTTL)

	// Update engine run ID index if it changed
	if run.EngineRunID != "" && run.EngineRunID != existing.EngineRunID {
		// Remove old index if it existed
		if existing.EngineRunID != "" {
			oldEngineKey := r.engineRunIDKey(existing.EngineRunID)
			pipe.Del(r.ctx, oldEngineKey)
		}
		// Create new index
		engineKey := r.engineRunIDKey(run.EngineRunID)
		pipe.Set(r.ctx, engineKey, run.RunID, runTTL)
	}

	_, err = pipe.Exec(r.ctx)
	if err != nil {
		return fmt.Errorf("failed to update run: %w", err)
	}

	// Publish update event to Redis Pub/Sub only if run has valid data
	// Skip publishing if run is empty/invalid (e.g., only has run_id)
	if run.RunID != "" && run.Status != "" {
		eventChannel := r.runEventChannel(run.RunID)
		eventData, err := json.Marshal(run)
		if err == nil {
			r.client.Publish(r.ctx, eventChannel, eventData)
		}
	}

	return nil
}

// ListByUserID retrieves all run IDs for a user
func (r *RunRepository) ListByUserID(userID string) ([]string, error) {
	userRunSetKey := r.userRunSetKey(userID)

	runIDs, err := r.client.SMembers(r.ctx, userRunSetKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list runs for user: %w", err)
	}

	return runIDs, nil
}

// Delete deletes a run
func (r *RunRepository) Delete(runID string) error {
	run, err := r.GetByRunID(runID)
	if err != nil {
		return err
	}

	runKey := r.runKey(runID)
	userRunSetKey := r.userRunSetKey(run.UserID)

	pipe := r.client.Pipeline()
	pipe.Del(r.ctx, runKey)
	pipe.SRem(r.ctx, userRunSetKey, runID)

	// Remove engine run ID index if it exists
	if run.EngineRunID != "" {
		engineKey := r.engineRunIDKey(run.EngineRunID)
		pipe.Del(r.ctx, engineKey)
	}

	_, err = pipe.Exec(r.ctx)
	if err != nil {
		return fmt.Errorf("failed to delete run: %w", err)
	}

	return nil
}

// Helper methods for key generation
func (r *RunRepository) runKey(runID string) string {
	return fmt.Sprintf("%s%s", runKeyPrefix, runID)
}

func (r *RunRepository) userRunSetKey(userID string) string {
	return fmt.Sprintf("%s%s:runs", userRunSetPrefix, userID)
}

func (r *RunRepository) engineRunIDKey(engineRunID string) string {
	return fmt.Sprintf("%s%s", engineRunIDPrefix, engineRunID)
}

func (r *RunRepository) runEventChannel(runID string) string {
	return fmt.Sprintf("%s%s", runEventChannelPrefix, runID)
}
