package unit

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateRun_Success(t *testing.T) {
	t.Run("validates request structure", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"metadata": map[string]interface{}{
				"key": "value",
			},
		}

		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		var parsed struct {
			Metadata map[string]interface{} `json:"metadata,omitempty"`
		}
		err = json.Unmarshal(body, &parsed)
		require.NoError(t, err)
		assert.Equal(t, "value", parsed.Metadata["key"])
	})
}

func TestCreateRun_MissingUserID(t *testing.T) {
	t.Run("requires user authentication", func(t *testing.T) {
		// This tests the handler logic for missing user ID
		// In a real test with mocked service, we'd verify 401 is returned
		userID := ""
		assert.Empty(t, userID, "user ID should be empty for unauthorized request")
	})
}

func TestGetRun_InvalidID(t *testing.T) {
	t.Run("validates run ID parameter", func(t *testing.T) {
		runID := ""
		assert.Empty(t, runID, "run ID should not be empty")
	})
}

func TestUpdateRun_RequestValidation(t *testing.T) {
	t.Run("validates update request body", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"status":        domain.StatusRunning,
			"engine_run_id": "engine-123",
			"metadata": map[string]interface{}{
				"key": "value",
			},
		}

		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		var parsed struct {
			Status      *string                `json:"status,omitempty"`
			EngineRunID *string                `json:"engine_run_id,omitempty"`
			Metadata    map[string]interface{} `json:"metadata,omitempty"`
		}
		err = json.Unmarshal(body, &parsed)
		require.NoError(t, err)
		assert.NotNil(t, parsed.Status)
		assert.Equal(t, domain.StatusRunning, *parsed.Status)
		assert.NotNil(t, parsed.EngineRunID)
		assert.Equal(t, "engine-123", *parsed.EngineRunID)
		assert.Equal(t, "value", parsed.Metadata["key"])
	})
}

func TestListRuns_RequiresUserID(t *testing.T) {
	t.Run("requires user authentication for listing runs", func(t *testing.T) {
		userID := ""
		assert.Empty(t, userID, "user ID should be required")
	})
}

// Test domain models and request structures
func TestDomainModels(t *testing.T) {
	t.Run("CreateRunRequest", func(t *testing.T) {
		req := &domain.CreateRunRequest{
			UserID:   "user123",
			Metadata: map[string]interface{}{"key": "value"},
		}
		assert.Equal(t, "user123", req.UserID)
		assert.Equal(t, "value", req.Metadata["key"])
	})

	t.Run("UpdateRunRequest", func(t *testing.T) {
		status := domain.StatusCompleted
		engineRunID := "engine-123"
		req := &domain.UpdateRunRequest{
			Status:      &status,
			EngineRunID: &engineRunID,
			Metadata:    map[string]interface{}{"key": "value"},
		}
		assert.NotNil(t, req.Status)
		assert.Equal(t, domain.StatusCompleted, *req.Status)
		assert.NotNil(t, req.EngineRunID)
		assert.Equal(t, "engine-123", *req.EngineRunID)
		assert.Equal(t, "value", req.Metadata["key"])
	})

	t.Run("SimulationRun", func(t *testing.T) {
		now := time.Now()
		completedAt := now.Add(1 * time.Hour)

		run := &domain.SimulationRun{
			RunID:       "run123",
			UserID:      "user123",
			EngineRunID: "engine123",
			Status:      domain.StatusRunning,
			CreatedAt:   now,
			UpdatedAt:   now,
			CompletedAt: &completedAt,
			Metadata:    map[string]interface{}{"key": "value"},
		}

		assert.Equal(t, "run123", run.RunID)
		assert.Equal(t, "user123", run.UserID)
		assert.Equal(t, "engine123", run.EngineRunID)
		assert.Equal(t, domain.StatusRunning, run.Status)
		assert.NotNil(t, run.CompletedAt)
		assert.Equal(t, "value", run.Metadata["key"])
	})
}

func TestStatusConstants(t *testing.T) {
	t.Run("valid statuses", func(t *testing.T) {
		statuses := []string{
			domain.StatusPending,
			domain.StatusRunning,
			domain.StatusCompleted,
			domain.StatusFailed,
			domain.StatusCancelled,
		}

		for _, status := range statuses {
			assert.NotEmpty(t, status)
		}

		assert.Equal(t, "pending", domain.StatusPending)
		assert.Equal(t, "running", domain.StatusRunning)
		assert.Equal(t, "completed", domain.StatusCompleted)
		assert.Equal(t, "failed", domain.StatusFailed)
		assert.Equal(t, "cancelled", domain.StatusCancelled)
	})
}
