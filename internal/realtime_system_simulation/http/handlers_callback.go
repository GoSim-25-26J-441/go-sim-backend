package http

import (
	"context"
	"crypto/subtle"
	"log"
	"net/http"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/domain"
	"github.com/gin-gonic/gin"
)

// EngineRunCallbackBody represents the callback payload from the simulation engine
type EngineRunCallbackBody struct {
	RunID           string                 `json:"run_id"`
	Status          interface{}            `json:"status"`           // Can be int (enum) or string
	StatusString    string                 `json:"status_string"`    // Preferred: string representation
	CreatedAtUnixMs int64                 `json:"created_at_unix_ms"`
	StartedAtUnixMs int64                 `json:"started_at_unix_ms"`
	EndedAtUnixMs   int64                 `json:"ended_at_unix_ms"`
	Metrics         map[string]interface{} `json:"metrics"`
	TimestampUnixMs int64                  `json:"timestamp"`
	Error           string                 `json:"error,omitempty"`
}

// EngineRunCallback handles callbacks from the simulation engine when a run changes state
// The callback is authenticated using header: X-Simulation-Callback-Secret (optional in dev if secret is not configured)
func (h *Handler) EngineRunCallback(c *gin.Context) {
	// Authn: shared secret (engine-to-backend)
	// If secret is configured, require it; otherwise allow (for local development)
	if h.callbackSecret != "" {
		secret := c.GetHeader("X-Simulation-Callback-Secret")
		if subtle.ConstantTimeCompare([]byte(secret), []byte(h.callbackSecret)) != 1 {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized: invalid callback secret"})
			return
		}
	}

	var body EngineRunCallbackBody
	if err := c.ShouldBindJSON(&body); err != nil {
		log.Printf("Callback JSON binding error: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	// Extract status string - prefer status_string, fallback to status (convert int to string if needed)
	statusStr := body.StatusString
	if statusStr == "" {
		switch v := body.Status.(type) {
		case string:
			statusStr = v
		case float64:
			statusStr = mapNumericStatusToString(int(v))
		case int:
			statusStr = mapNumericStatusToString(v)
		}
	}

	// Find the run by engine run ID
	run, err := h.simService.GetRunByEngineID(body.RunID)
	if err != nil {
		log.Printf("Callback: run not found for engine_run_id=%s: %v", body.RunID, err)
		c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
		return
	}

	// Update run status
	updateReq := &domain.UpdateRunRequest{
		Status: &statusStr,
	}

	// Merge metrics into metadata if provided
	if len(body.Metrics) > 0 {
		if run.Metadata == nil {
			run.Metadata = make(map[string]interface{})
		}
		for k, v := range body.Metrics {
			run.Metadata[k] = v
		}
		updateReq.Metadata = run.Metadata
	}

	updatedRun, err := h.simService.UpdateRun(run.RunID, updateReq)
	if err != nil {
		log.Printf("Callback: failed to update run_id=%s: %v", run.RunID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update run"})
		return
	}

	// Trigger persistence when run completes successfully
	if statusStr == domain.StatusCompleted && updatedRun.EngineRunID != "" {
		go func() {
			ctx := context.Background()
			if err := h.simService.StoreRunSummaryAndMetrics(ctx, updatedRun.RunID); err != nil {
				log.Printf("Callback: Failed to persist summary and metrics for run_id=%s: %v", updatedRun.RunID, err)
			} else {
				log.Printf("Callback: Successfully persisted summary and metrics for run_id=%s", updatedRun.RunID)
			}
		}()
	}

	c.JSON(http.StatusOK, gin.H{"message": "callback processed", "run_id": updatedRun.RunID})
}

// EngineRunCallbackByID handles callbacks with run ID in URL path
func (h *Handler) EngineRunCallbackByID(c *gin.Context) {
	runID := c.Param("id")
	if runID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "run ID is required"})
		return
	}

	// Authn: shared secret
	if h.callbackSecret != "" {
		secret := c.GetHeader("X-Simulation-Callback-Secret")
		if subtle.ConstantTimeCompare([]byte(secret), []byte(h.callbackSecret)) != 1 {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized: invalid callback secret"})
			return
		}
	}

	var body EngineRunCallbackBody
	if err := c.ShouldBindJSON(&body); err != nil {
		log.Printf("Callback JSON binding error for run_id=%s: %v", runID, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	// Get the run
	run, err := h.simService.GetRun(runID)
	if err != nil {
		log.Printf("Callback: run not found for run_id=%s: %v", runID, err)
		c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
		return
	}

	// Extract status string
	statusStr := body.StatusString
	if statusStr == "" {
		switch v := body.Status.(type) {
		case string:
			statusStr = v
		case float64:
			statusStr = mapNumericStatusToString(int(v))
		case int:
			statusStr = mapNumericStatusToString(v)
		}
	}

	// Update run status
	updateReq := &domain.UpdateRunRequest{
		Status: &statusStr,
	}

	// Merge metrics into metadata if provided
	if len(body.Metrics) > 0 {
		if run.Metadata == nil {
			run.Metadata = make(map[string]interface{})
		}
		for k, v := range body.Metrics {
			run.Metadata[k] = v
		}
		updateReq.Metadata = run.Metadata
	}

	updatedRun, err := h.simService.UpdateRun(runID, updateReq)
	if err != nil {
		log.Printf("Callback: failed to update run_id=%s: %v", runID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update run"})
		return
	}

	// Trigger persistence when run completes successfully
	if statusStr == domain.StatusCompleted && updatedRun.EngineRunID != "" {
		go func() {
			ctx := context.Background()
			if err := h.simService.StoreRunSummaryAndMetrics(ctx, runID); err != nil {
				log.Printf("Callback: Failed to persist summary and metrics for run_id=%s: %v", runID, err)
			} else {
				log.Printf("Callback: Successfully persisted summary and metrics for run_id=%s", runID)
			}
		}()
	}

	c.JSON(http.StatusOK, gin.H{"message": "callback processed", "run_id": runID})
}

// mapNumericStatusToString converts numeric status codes to string status
func mapNumericStatusToString(status int) string {
	switch status {
	case 0:
		return domain.StatusPending
	case 1:
		return domain.StatusRunning
	case 2:
		return domain.StatusCompleted
	case 3:
		return domain.StatusFailed
	case 4:
		return domain.StatusCancelled
	default:
		return domain.StatusPending
	}
}
