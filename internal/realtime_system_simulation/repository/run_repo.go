package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/domain"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	runKeyPrefix          = "sim:run:"         // Key prefix for run data: sim:run:{run_id}
	userRunSetPrefix      = "sim:user:"        // Set of run IDs for a user: sim:user:{user_id}:runs
	projectRunSetPrefix   = "sim:project:"     // Set of run IDs for a project: sim:project:{project_id}:runs
	engineRunIDPrefix     = "sim:engine:"      // Mapping from engine run ID to our run ID: sim:engine:{engine_run_id} -> run_id
	runEventChannelPrefix = "sim:events:"      // Pub/Sub channel for run events: sim:events:{run_id}
	runTTL                = 7 * 24 * time.Hour // TTL for run data (7 days)
)

// RunRepository handles durable simulation run metadata and Redis run events/cache.
type RunRepository struct {
	client *redis.Client
	db     *sql.DB
	ctx    context.Context
}

// NewRunRepository creates a new RunRepository
func NewRunRepository(client *redis.Client) *RunRepository {
	return &RunRepository{
		client: client,
		ctx:    context.Background(),
	}
}

// NewRunRepositoryWithDB creates a run repository that persists run metadata in
// PostgreSQL while keeping Redis as a cache and Pub/Sub transport.
func NewRunRepositoryWithDB(client *redis.Client, db *sql.DB) *RunRepository {
	return &RunRepository{
		client: client,
		db:     db,
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

	if r.db != nil {
		if err := r.upsertRunInPostgres(r.ctx, run); err != nil {
			return err
		}
	}

	return r.createRunInRedis(run)
}

func (r *RunRepository) createRunInRedis(run *domain.SimulationRun) error {
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

	// Index by project if associated
	if run.ProjectPublicID != "" {
		projectRunSetKey := r.projectRunSetKey(run.ProjectPublicID)
		pipe.SAdd(r.ctx, projectRunSetKey, run.RunID)
		pipe.Expire(r.ctx, projectRunSetKey, runTTL)
	}

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
	if r.db != nil {
		run, err := r.getRunFromPostgres(r.ctx, `WHERE run_id = $1`, runID)
		if err == nil {
			return run, nil
		}
		if err != domain.ErrRunNotFound {
			return nil, err
		}
	}

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
	if r.db != nil {
		run, err := r.getRunFromPostgres(r.ctx, `WHERE engine_run_id = $1`, engineRunID)
		if err == nil {
			return run, nil
		}
		if err != domain.ErrRunNotFound {
			return nil, err
		}
	}

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

	if r.db != nil {
		if err := r.upsertRunInPostgres(r.ctx, run); err != nil {
			return err
		}
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
	if r.db != nil {
		return r.listRunIDsFromPostgres(r.ctx, `WHERE user_id = $1`, userID)
	}

	userRunSetKey := r.userRunSetKey(userID)

	runIDs, err := r.client.SMembers(r.ctx, userRunSetKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list runs for user: %w", err)
	}

	return runIDs, nil
}

// ListByProjectID retrieves all run IDs for a project
func (r *RunRepository) ListByProjectID(projectPublicID string) ([]string, error) {
	if r.db != nil {
		return r.listRunIDsFromPostgres(r.ctx, `WHERE project_public_id = $1`, projectPublicID)
	}

	projectRunSetKey := r.projectRunSetKey(projectPublicID)

	runIDs, err := r.client.SMembers(r.ctx, projectRunSetKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list runs for project: %w", err)
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

	// Remove from project index if associated
	if run.ProjectPublicID != "" {
		projectRunSetKey := r.projectRunSetKey(run.ProjectPublicID)
		pipe.SRem(r.ctx, projectRunSetKey, runID)
	}

	// Remove engine run ID index if it exists
	if run.EngineRunID != "" {
		engineKey := r.engineRunIDKey(run.EngineRunID)
		pipe.Del(r.ctx, engineKey)
	}

	_, err = pipe.Exec(r.ctx)
	if err != nil {
		return fmt.Errorf("failed to delete run: %w", err)
	}

	if r.db != nil {
		if _, err := r.db.ExecContext(r.ctx, `DELETE FROM simulation_runs WHERE run_id = $1`, runID); err != nil {
			return fmt.Errorf("failed to delete run from postgres: %w", err)
		}
	}

	return nil
}

func (r *RunRepository) upsertRunInPostgres(ctx context.Context, run *domain.SimulationRun) error {
	if r.db == nil {
		return nil
	}

	metadata := run.Metadata
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal run metadata: %w", err)
	}

	_, err = r.db.ExecContext(
		ctx,
		`INSERT INTO simulation_runs (
			run_id, user_id, project_public_id, engine_run_id, status,
			created_at, updated_at, completed_at, metadata
		)
		VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), NULLIF($4, ''), $5, $6, $7, $8, $9::jsonb)
		ON CONFLICT (run_id) DO UPDATE SET
			user_id = COALESCE(EXCLUDED.user_id, simulation_runs.user_id),
			project_public_id = EXCLUDED.project_public_id,
			engine_run_id = COALESCE(EXCLUDED.engine_run_id, simulation_runs.engine_run_id),
			status = EXCLUDED.status,
			updated_at = EXCLUDED.updated_at,
			completed_at = EXCLUDED.completed_at,
			metadata = EXCLUDED.metadata`,
		run.RunID,
		run.UserID,
		run.ProjectPublicID,
		run.EngineRunID,
		run.Status,
		run.CreatedAt,
		run.UpdatedAt,
		run.CompletedAt,
		string(metadataJSON),
	)
	if err != nil {
		return fmt.Errorf("failed to upsert run in postgres: %w", err)
	}

	return nil
}

func (r *RunRepository) getRunFromPostgres(ctx context.Context, where string, arg interface{}) (*domain.SimulationRun, error) {
	query := fmt.Sprintf(
		`SELECT run_id,
			COALESCE(user_id, ''),
			COALESCE(project_public_id, ''),
			COALESCE(engine_run_id, ''),
			COALESCE(status, $2),
			created_at,
			COALESCE(updated_at, created_at),
			completed_at,
			COALESCE(metadata, '{}'::jsonb)
		FROM simulation_runs
		%s`,
		where,
	)

	var run domain.SimulationRun
	var metadataJSON []byte
	err := r.db.QueryRowContext(ctx, query, arg, domain.StatusPending).Scan(
		&run.RunID,
		&run.UserID,
		&run.ProjectPublicID,
		&run.EngineRunID,
		&run.Status,
		&run.CreatedAt,
		&run.UpdatedAt,
		&run.CompletedAt,
		&metadataJSON,
	)
	if err == sql.ErrNoRows {
		return nil, domain.ErrRunNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get run from postgres: %w", err)
	}

	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &run.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal run metadata: %w", err)
		}
	}
	if run.Metadata == nil {
		run.Metadata = map[string]interface{}{}
	}

	return &run, nil
}

func (r *RunRepository) listRunIDsFromPostgres(ctx context.Context, where string, arg interface{}) ([]string, error) {
	query := fmt.Sprintf(
		`SELECT run_id
		FROM simulation_runs
		%s
		ORDER BY created_at DESC`,
		where,
	)

	rows, err := r.db.QueryContext(ctx, query, arg)
	if err != nil {
		return nil, fmt.Errorf("failed to list runs from postgres: %w", err)
	}
	defer rows.Close()

	var runIDs []string
	for rows.Next() {
		var runID string
		if err := rows.Scan(&runID); err != nil {
			return nil, fmt.Errorf("failed to scan run ID from postgres: %w", err)
		}
		runIDs = append(runIDs, runID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate postgres run IDs: %w", err)
	}

	return runIDs, nil
}

// Helper methods for key generation
func (r *RunRepository) runKey(runID string) string {
	return fmt.Sprintf("%s%s", runKeyPrefix, runID)
}

func (r *RunRepository) userRunSetKey(userID string) string {
	return fmt.Sprintf("%s%s:runs", userRunSetPrefix, userID)
}

func (r *RunRepository) projectRunSetKey(projectPublicID string) string {
	return fmt.Sprintf("%s%s:runs", projectRunSetPrefix, projectPublicID)
}

func (r *RunRepository) engineRunIDKey(engineRunID string) string {
	return fmt.Sprintf("%s%s", engineRunIDPrefix, engineRunID)
}

func (r *RunRepository) runEventChannel(runID string) string {
	return fmt.Sprintf("%s%s", runEventChannelPrefix, runID)
}
