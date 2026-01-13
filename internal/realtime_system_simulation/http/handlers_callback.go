package http

import (
	"crypto/subtle"
	"log"
	"net/http"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/domain"
	"github.com/gin-gonic/gin"
)

type engineRunCallbackBody struct {
	RunID            string                 `json:"run_id"`
	Status           interface{}            `json:"status"`           // Can be int (enum) or string
	StatusString     string                 `json:"status_string"`    // Preferred: string representation
	CreatedAtUnixMs  int64                  `json:"created_at_unix_ms"`
	StartedAtUnixMs  int64                  `json:"started_at_unix_ms"`
	EndedAtUnixMs    int64                  `json:"ended_at_unix_ms"`
	Metrics          map[string]interface{} `json:"metrics"`
	TimestampUnixMs  int64                  `json:"timestamp"`
	Error            string                 `json:"error,omitempty"`  // Error message if any
}

// EngineRunCallback handles callbacks from the simulation engine when a run changes state.
// The callback is authenticated using header: X-Simulation-Callback-Secret (optional in dev if secret is not configured).
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
	// If no secret configured, allow all callbacks (development mode)

	var body engineRunCallbackBody
	if err := c.ShouldBindJSON(&body); err != nil {
		log.Printf("Callback JSON binding error (legacy): %v", err)
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
	if body.RunID == "" || statusStr == "" {
		log.Printf("Callback missing run_id or status (legacy) for run_id=%s", body.RunID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "run_id and status/status_string are required"})
		return
	}

	// Find run: in our integration, engine may use our backend run_id as its run_id, but we support both.
	run, err := h.simService.GetRun(body.RunID)
	if err != nil {
		run, err = h.simService.GetRunByEngineID(body.RunID)
		if err != nil {
			if err == domain.ErrRunNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get run"})
			return
		}
	}

	backendStatus := mapEngineStatusToBackendStatus(statusStr)
	
	// Preserve existing metadata and merge callback data
	updateMeta := make(map[string]interface{})
	if run.Metadata != nil {
		// Copy existing metadata to preserve it
		for k, v := range run.Metadata {
			updateMeta[k] = v
		}
	}
	
	// Add/update callback-related metadata
	updateMeta["engine_status"] = statusStr
	updateMeta["engine_status_string"] = body.StatusString
	updateMeta["engine_created_at_unix_ms"] = body.CreatedAtUnixMs
	updateMeta["engine_started_at_unix_ms"] = body.StartedAtUnixMs
	updateMeta["engine_ended_at_unix_ms"] = body.EndedAtUnixMs
	updateMeta["engine_callback_ts_unix_ms"] = body.TimestampUnixMs
	
	if body.Error != "" {
		updateMeta["engine_error"] = body.Error
	}
	
	// Add metrics if provided
	if len(body.Metrics) > 0 {
		updateMeta["metrics"] = body.Metrics
		log.Printf("Callback received metrics for run_id=%s (legacy): %d metric fields", body.RunID, len(body.Metrics))
	} else {
		log.Printf("Callback received NO metrics for run_id=%s (legacy) - metrics field is empty", body.RunID)
	}

	updated, err := h.simService.UpdateRun(run.RunID, &domain.UpdateRunRequest{
		Status:   &backendStatus,
		Metadata: updateMeta,
	})
	if err != nil {
		if err == domain.ErrInvalidStatus {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update run"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "run": updated})
}

// EngineRunCallbackByID handles callbacks with run_id in the URL path (more secure).
// This is the recommended approach as it uniquely identifies the run via the URL.
func (h *Handler) EngineRunCallbackByID(c *gin.Context) {
	// Get run_id from URL path
	runIDFromURL := c.Param("run_id")
	if runIDFromURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "run_id is required in URL path"})
		return
	}

	// Authn: shared secret (engine-to-backend)
	// If secret is configured, require it; otherwise allow (for local development)
	if h.callbackSecret != "" {
		secret := c.GetHeader("X-Simulation-Callback-Secret")
		if subtle.ConstantTimeCompare([]byte(secret), []byte(h.callbackSecret)) != 1 {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized: invalid callback secret"})
			return
		}
	}

	var body engineRunCallbackBody
	if err := c.ShouldBindJSON(&body); err != nil {
		log.Printf("Callback JSON binding error for run_id=%s: %v", runIDFromURL, err)
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
			// Map numeric status codes to strings
			statusStr = mapNumericStatusToString(int(v))
		case int:
			statusStr = mapNumericStatusToString(v)
		}
	}
	if statusStr == "" {
		log.Printf("Callback missing status field for run_id=%s", runIDFromURL)
		c.JSON(http.StatusBadRequest, gin.H{"error": "status or status_string is required"})
		return
	}

	// Use run_id from URL (more secure - URL identifies the run)
	// Optionally validate that run_id in body matches URL if provided
	if body.RunID != "" && body.RunID != runIDFromURL {
		c.JSON(http.StatusBadRequest, gin.H{"error": "run_id in URL does not match run_id in body"})
		return
	}

	// Find run by run_id from URL
	run, err := h.simService.GetRun(runIDFromURL)
	if err != nil {
		// Try engine_run_id as fallback
		run, err = h.simService.GetRunByEngineID(runIDFromURL)
		if err != nil {
			if err == domain.ErrRunNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get run"})
			return
		}
	}

	backendStatus := mapEngineStatusToBackendStatus(statusStr)
	
	// Preserve existing metadata and merge callback data
	updateMeta := make(map[string]interface{})
	if run.Metadata != nil {
		// Copy existing metadata to preserve it
		for k, v := range run.Metadata {
			updateMeta[k] = v
		}
	}
	
	// Add/update callback-related metadata
	updateMeta["engine_status"] = statusStr
	updateMeta["engine_status_string"] = body.StatusString
	updateMeta["engine_created_at_unix_ms"] = body.CreatedAtUnixMs
	updateMeta["engine_started_at_unix_ms"] = body.StartedAtUnixMs
	updateMeta["engine_ended_at_unix_ms"] = body.EndedAtUnixMs
	updateMeta["engine_callback_ts_unix_ms"] = body.TimestampUnixMs
	
	if body.Error != "" {
		updateMeta["engine_error"] = body.Error
	}
	
	// Add metrics if provided
	if len(body.Metrics) > 0 {
		updateMeta["metrics"] = body.Metrics
		log.Printf("Callback received metrics for run_id=%s: %d metric fields", runIDFromURL, len(body.Metrics))
	} else {
		log.Printf("Callback received NO metrics for run_id=%s - metrics field is empty", runIDFromURL)
	}

	updated, err := h.simService.UpdateRun(run.RunID, &domain.UpdateRunRequest{
		Status:   &backendStatus,
		Metadata: updateMeta,
	})
	if err != nil {
		if err == domain.ErrInvalidStatus {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update run"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "run": updated})
}

// mapNumericStatusToString converts numeric status codes from simulator to string
func mapNumericStatusToString(statusCode int) string {
	switch statusCode {
	case 0, 1:
		return "RUN_STATUS_PENDING"
	case 2:
		return "RUN_STATUS_RUNNING"
	case 3:
		return "RUN_STATUS_COMPLETED"
	case 4:
		return "RUN_STATUS_FAILED"
	case 5:
		return "RUN_STATUS_CANCELLED"
	default:
		return "RUN_STATUS_PENDING"
	}
}

func mapEngineStatusToBackendStatus(engineStatus string) string {
	switch engineStatus {
	case "RUN_STATUS_PENDING":
		return domain.StatusPending
	case "RUN_STATUS_RUNNING":
		return domain.StatusRunning
	case "RUN_STATUS_COMPLETED":
		return domain.StatusCompleted
	case "RUN_STATUS_FAILED":
		return domain.StatusFailed
	case "RUN_STATUS_CANCELLED":
		return domain.StatusCancelled
	default:
		return domain.StatusPending
	}
}


