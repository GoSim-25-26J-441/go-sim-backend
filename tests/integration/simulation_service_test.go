package integration

import (
	"context"
	"testing"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/domain"
	simrepo "github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/repository"
	simservice "github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	// Test connection
	ctx := context.Background()
	err = client.Ping(ctx).Err()
	require.NoError(t, err)

	return client, mr
}

func TestSimulationService_CreateRun(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	repo := simrepo.NewRunRepository(client)
	service := simservice.NewSimulationService(repo)

	t.Run("creates run successfully", func(t *testing.T) {
		req := &domain.CreateRunRequest{
			UserID:   "user123",
			Metadata: map[string]interface{}{"key": "value"},
		}

		run, err := service.CreateRun(req)
		require.NoError(t, err)
		assert.NotEmpty(t, run.RunID)
		assert.Equal(t, "user123", run.UserID)
		assert.Equal(t, domain.StatusPending, run.Status)
		assert.Equal(t, "value", run.Metadata["key"])
		assert.False(t, run.CreatedAt.IsZero())
		assert.False(t, run.UpdatedAt.IsZero())
	})

	t.Run("creates run without metadata", func(t *testing.T) {
		req := &domain.CreateRunRequest{
			UserID: "user456",
		}

		run, err := service.CreateRun(req)
		require.NoError(t, err)
		assert.NotEmpty(t, run.RunID)
		assert.Equal(t, "user456", run.UserID)
		assert.NotNil(t, run.Metadata)
		assert.Equal(t, 0, len(run.Metadata))
	})
}

func TestSimulationService_GetRun(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	repo := simrepo.NewRunRepository(client)
	service := simservice.NewSimulationService(repo)

	t.Run("gets run by ID", func(t *testing.T) {
		// Create a run first
		req := &domain.CreateRunRequest{
			UserID: "user123",
		}
		created, err := service.CreateRun(req)
		require.NoError(t, err)

		// Get the run
		run, err := service.GetRun(created.RunID)
		require.NoError(t, err)
		assert.Equal(t, created.RunID, run.RunID)
		assert.Equal(t, "user123", run.UserID)
	})

	t.Run("returns error for non-existent run", func(t *testing.T) {
		_, err := service.GetRun("non-existent-run-id")
		assert.Error(t, err)
		assert.Equal(t, domain.ErrRunNotFound, err)
	})
}

func TestSimulationService_UpdateRun(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	repo := simrepo.NewRunRepository(client)
	service := simservice.NewSimulationService(repo)

	t.Run("updates run status", func(t *testing.T) {
		// Create a run
		req := &domain.CreateRunRequest{
			UserID: "user123",
		}
		created, err := service.CreateRun(req)
		require.NoError(t, err)

		// Update status
		status := domain.StatusRunning
		updateReq := &domain.UpdateRunRequest{
			Status: &status,
		}
		updated, err := service.UpdateRun(created.RunID, updateReq)
		require.NoError(t, err)
		assert.Equal(t, domain.StatusRunning, updated.Status)
	})

	t.Run("updates engine run ID", func(t *testing.T) {
		// Create a run
		req := &domain.CreateRunRequest{
			UserID: "user123",
		}
		created, err := service.CreateRun(req)
		require.NoError(t, err)

		// Update engine run ID
		engineRunID := "engine-123"
		updateReq := &domain.UpdateRunRequest{
			EngineRunID: &engineRunID,
		}
		updated, err := service.UpdateRun(created.RunID, updateReq)
		require.NoError(t, err)
		assert.Equal(t, "engine-123", updated.EngineRunID)
	})

	t.Run("updates metadata", func(t *testing.T) {
		// Create a run
		req := &domain.CreateRunRequest{
			UserID: "user123",
		}
		created, err := service.CreateRun(req)
		require.NoError(t, err)

		// Update metadata
		updateReq := &domain.UpdateRunRequest{
			Metadata: map[string]interface{}{"key": "value"},
		}
		updated, err := service.UpdateRun(created.RunID, updateReq)
		require.NoError(t, err)
		assert.Equal(t, "value", updated.Metadata["key"])
	})

	t.Run("sets completed_at when status is completed", func(t *testing.T) {
		// Create a run
		req := &domain.CreateRunRequest{
			UserID: "user123",
		}
		created, err := service.CreateRun(req)
		require.NoError(t, err)

		// Update to completed status
		status := domain.StatusCompleted
		updateReq := &domain.UpdateRunRequest{
			Status: &status,
		}
		updated, err := service.UpdateRun(created.RunID, updateReq)
		require.NoError(t, err)
		assert.Equal(t, domain.StatusCompleted, updated.Status)
		assert.NotNil(t, updated.CompletedAt)
	})

	t.Run("sets completed_at when status is failed", func(t *testing.T) {
		// Create a run
		req := &domain.CreateRunRequest{
			UserID: "user123",
		}
		created, err := service.CreateRun(req)
		require.NoError(t, err)

		// Update to failed status
		status := domain.StatusFailed
		updateReq := &domain.UpdateRunRequest{
			Status: &status,
		}
		updated, err := service.UpdateRun(created.RunID, updateReq)
		require.NoError(t, err)
		assert.Equal(t, domain.StatusFailed, updated.Status)
		assert.NotNil(t, updated.CompletedAt)
	})

	t.Run("returns error for invalid status", func(t *testing.T) {
		// Create a run
		req := &domain.CreateRunRequest{
			UserID: "user123",
		}
		created, err := service.CreateRun(req)
		require.NoError(t, err)

		// Try to update with invalid status
		invalidStatus := "invalid-status"
		updateReq := &domain.UpdateRunRequest{
			Status: &invalidStatus,
		}
		_, err = service.UpdateRun(created.RunID, updateReq)
		assert.Error(t, err)
		assert.Equal(t, domain.ErrInvalidStatus, err)
	})

	t.Run("returns error for non-existent run", func(t *testing.T) {
		status := domain.StatusRunning
		updateReq := &domain.UpdateRunRequest{
			Status: &status,
		}
		_, err := service.UpdateRun("non-existent-run-id", updateReq)
		assert.Error(t, err)
		assert.Equal(t, domain.ErrRunNotFound, err)
	})
}

func TestSimulationService_GetRunByEngineID(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	repo := simrepo.NewRunRepository(client)
	service := simservice.NewSimulationService(repo)

	t.Run("gets run by engine run ID", func(t *testing.T) {
		// Create a run
		req := &domain.CreateRunRequest{
			UserID: "user123",
		}
		created, err := service.CreateRun(req)
		require.NoError(t, err)

		// Set engine run ID
		engineRunID := "engine-123"
		updateReq := &domain.UpdateRunRequest{
			EngineRunID: &engineRunID,
		}
		_, err = service.UpdateRun(created.RunID, updateReq)
		require.NoError(t, err)

		// Get by engine run ID
		run, err := service.GetRunByEngineID("engine-123")
		require.NoError(t, err)
		assert.Equal(t, created.RunID, run.RunID)
		assert.Equal(t, "engine-123", run.EngineRunID)
	})

	t.Run("returns error for non-existent engine run ID", func(t *testing.T) {
		_, err := service.GetRunByEngineID("non-existent-engine-id")
		assert.Error(t, err)
		assert.Equal(t, domain.ErrRunNotFound, err)
	})
}

func TestSimulationService_ListRunsByUser(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	repo := simrepo.NewRunRepository(client)
	service := simservice.NewSimulationService(repo)

	t.Run("lists runs for user", func(t *testing.T) {
		userID := "user123"

		// Create multiple runs
		req1 := &domain.CreateRunRequest{UserID: userID}
		run1, err := service.CreateRun(req1)
		require.NoError(t, err)

		req2 := &domain.CreateRunRequest{UserID: userID}
		run2, err := service.CreateRun(req2)
		require.NoError(t, err)

		// List runs
		runIDs, err := service.ListRunsByUser(userID)
		require.NoError(t, err)
		assert.Contains(t, runIDs, run1.RunID)
		assert.Contains(t, runIDs, run2.RunID)
	})

	t.Run("returns empty list for user with no runs", func(t *testing.T) {
		runIDs, err := service.ListRunsByUser("user-with-no-runs")
		require.NoError(t, err)
		assert.NotNil(t, runIDs)
		assert.Equal(t, 0, len(runIDs))
	})
}

func TestSimulationService_DeleteRun(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	repo := simrepo.NewRunRepository(client)
	service := simservice.NewSimulationService(repo)

	t.Run("deletes run successfully", func(t *testing.T) {
		// Create a run
		req := &domain.CreateRunRequest{
			UserID: "user123",
		}
		created, err := service.CreateRun(req)
		require.NoError(t, err)

		// Delete the run
		err = service.DeleteRun(created.RunID)
		require.NoError(t, err)

		// Verify it's deleted
		_, err = service.GetRun(created.RunID)
		assert.Error(t, err)
		assert.Equal(t, domain.ErrRunNotFound, err)
	})

	t.Run("returns error for non-existent run", func(t *testing.T) {
		err := service.DeleteRun("non-existent-run-id")
		assert.Error(t, err)
		assert.Equal(t, domain.ErrRunNotFound, err)
	})
}

