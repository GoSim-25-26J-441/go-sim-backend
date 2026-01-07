package http

import (
	"crypto/subtle"
	"net/http"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/domain"
	"github.com/gin-gonic/gin"
)

type engineRunCallbackBody struct {
	RunID            string                 `json:"run_id"`
	Status           string                 `json:"status"`
	StatusString     string                 `json:"status_string"`
	CreatedAtUnixMs  int64                  `json:"created_at_unix_ms"`
	StartedAtUnixMs  int64                  `json:"started_at_unix_ms"`
	EndedAtUnixMs    int64                  `json:"ended_at_unix_ms"`
	Metrics          map[string]interface{} `json:"metrics"`
	TimestampUnixMs  int64                  `json:"timestamp"`
}

// EngineRunCallback handles callbacks from the simulation engine when a run changes state.
// The callback is authenticated using header: X-Simulation-Callback-Secret.
func (h *Handler) EngineRunCallback(c *gin.Context) {
	// Authn: shared secret (engine-to-backend)
	secret := c.GetHeader("X-Simulation-Callback-Secret")
	if h.callbackSecret == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(h.callbackSecret)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var body engineRunCallbackBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if body.RunID == "" || body.Status == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "run_id and status are required"})
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

	backendStatus := mapEngineStatusToBackendStatus(body.Status)
	updateMeta := map[string]interface{}{
		"engine_status":             body.Status,
		"engine_status_string":      body.StatusString,
		"engine_created_at_unix_ms": body.CreatedAtUnixMs,
		"engine_started_at_unix_ms": body.StartedAtUnixMs,
		"engine_ended_at_unix_ms":   body.EndedAtUnixMs,
		"engine_callback_ts_unix_ms": body.TimestampUnixMs,
	}
	if body.Metrics != nil {
		updateMeta["metrics"] = body.Metrics
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


