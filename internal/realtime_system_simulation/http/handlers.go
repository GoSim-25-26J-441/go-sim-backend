package http

import (
	"log"
	"net/http"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/domain"
	"github.com/gin-gonic/gin"
)

// CreateRun creates a new simulation run
func (h *Handler) CreateRun(c *gin.Context) {
	// Get user ID from context (set by Firebase auth middleware if authenticated)
	userID := c.GetString("firebase_uid")
	if userID == "" {
		// Fallback to header or default for development
		userID = c.GetHeader("X-User-Id")
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
			return
		}
	}

	var body struct {
		ScenarioYAML string                 `json:"scenario_yaml,omitempty"`
		DurationMs   int64                  `json:"duration_ms,omitempty"`
		Metadata     map[string]interface{} `json:"metadata,omitempty"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Create run in backend first
	req := &domain.CreateRunRequest{
		UserID:   userID,
		Metadata: body.Metadata,
	}

	run, err := h.simService.CreateRun(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create run"})
		return
	}

	// If scenario_yaml is provided, create run in simulation engine
	if body.ScenarioYAML != "" && body.DurationMs > 0 {
		// Generate unique callback URL per run (includes run_id in path for identification)
		var callbackURL string
		if h.callbackURL != "" {
			// Append run_id to callback URL path: /callback/{run_id}
			callbackURL = h.callbackURL + "/" + run.RunID
			log.Printf("Creating run in simulation engine with unique callback URL: %s", callbackURL)
		} else {
			log.Printf("Warning: SIMULATION_CALLBACK_URL not set - simulation engine will not call back when run completes")
		}
		engineRunID, err := h.engineClient.CreateRun(run.RunID, body.ScenarioYAML, body.DurationMs, callbackURL, h.callbackSecret)
		if err != nil {
			// Log error but don't fail the request - the run is already created in backend
			// The user can retry by updating the run
			c.JSON(http.StatusCreated, gin.H{
				"run":     run,
				"warning": "run created in backend but failed to create in simulation engine: " + err.Error(),
			})
			return
		}

		// Update run with engine run ID
		engineRunIDPtr := &engineRunID
		run, err = h.simService.UpdateRun(run.RunID, &domain.UpdateRunRequest{
			EngineRunID: engineRunIDPtr,
		})
		if err != nil {
			// Log error but return the run (engine run ID is set in engine)
			c.JSON(http.StatusCreated, gin.H{
				"run":     run,
				"warning": "run created in engine but failed to update engine_run_id in backend",
			})
			return
		}
	}

	c.JSON(http.StatusCreated, gin.H{"run": run})
}

// GetRun retrieves a simulation run by ID
func (h *Handler) GetRun(c *gin.Context) {
	runID := c.Param("id")
	if runID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "run ID is required"})
		return
	}

	run, err := h.simService.GetRun(runID)
	if err != nil {
		if err == domain.ErrRunNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get run"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"run": run})
}

// GetRunByEngineID retrieves a simulation run by engine run ID
func (h *Handler) GetRunByEngineID(c *gin.Context) {
	engineRunID := c.Param("engine_run_id")
	if engineRunID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "engine run ID is required"})
		return
	}

	run, err := h.simService.GetRunByEngineID(engineRunID)
	if err != nil {
		if err == domain.ErrRunNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get run"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"run": run})
}

// UpdateRun updates a simulation run
func (h *Handler) UpdateRun(c *gin.Context) {
	runID := c.Param("id")
	if runID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "run ID is required"})
		return
	}

	var body struct {
		Status      *string                `json:"status,omitempty"`
		EngineRunID *string                `json:"engine_run_id,omitempty"`
		Metadata    map[string]interface{} `json:"metadata,omitempty"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Get the run first to check if it has an engine_run_id
	run, err := h.simService.GetRun(runID)
	if err != nil {
		if err == domain.ErrRunNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get run"})
		return
	}

	// If status is being set to "running" and we have an engine_run_id, start the run in the engine
	if body.Status != nil && *body.Status == domain.StatusRunning && run.EngineRunID != "" {
		err := h.engineClient.StartRun(run.EngineRunID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "failed to start run in simulation engine: " + err.Error(),
			})
			return
		}
	}

	// If status is being set to "cancelled" and we have an engine_run_id, stop the run in the engine
	if body.Status != nil && *body.Status == domain.StatusCancelled && run.EngineRunID != "" {
		err := h.engineClient.StopRun(run.EngineRunID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "failed to stop run in simulation engine: " + err.Error(),
			})
			return
		}
	}

	req := &domain.UpdateRunRequest{
		Status:      body.Status,
		EngineRunID: body.EngineRunID,
		Metadata:    body.Metadata,
	}

	run, err = h.simService.UpdateRun(runID, req)
	if err != nil {
		if err == domain.ErrRunNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
			return
		}
		if err == domain.ErrInvalidStatus {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update run"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"run": run})
}

// ListRuns lists all runs for the current user
func (h *Handler) ListRuns(c *gin.Context) {
	// Get user ID from context
	userID := c.GetString("firebase_uid")
	if userID == "" {
		userID = c.GetHeader("X-User-Id")
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
			return
		}
	}

	runIDs, err := h.simService.ListRunsByUser(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list runs"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"runs": runIDs})
}

// DeleteRun deletes a simulation run
func (h *Handler) DeleteRun(c *gin.Context) {
	runID := c.Param("id")
	if runID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "run ID is required"})
		return
	}

	if err := h.simService.DeleteRun(runID); err != nil {
		if err == domain.ErrRunNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete run"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "run deleted successfully"})
}
