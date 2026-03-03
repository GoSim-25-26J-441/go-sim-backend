package http

import (
	"database/sql"
	"log"
	"net/http"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/domain"
	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

// CreateRunForProject creates a new simulation run for a project (project_id in path)
func (h *Handler) CreateRunForProject(c *gin.Context) {
	projectID := c.Param("project_id")
	if projectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project_id is required"})
		return
	}
	userID := c.GetString("firebase_uid")
	if userID == "" {
		userID = c.GetHeader("X-User-Id")
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
			return
		}
	}

	var body struct {
		ScenarioYAML string                 `json:"scenario_yaml,omitempty"`
		DurationMs   int64                  `json:"duration_ms,omitempty"`
		RealTimeMode *bool                  `json:"real_time_mode,omitempty"`
		Metadata     map[string]interface{} `json:"metadata,omitempty"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	req := &domain.CreateRunRequest{
		UserID:          userID,
		ProjectPublicID: projectID,
		Metadata:        body.Metadata,
	}
	run, err := h.simService.CreateRun(req)
	if err != nil {
		log.Printf("Failed to create run in service: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create run", "details": err.Error()})
		return
	}

	if body.ScenarioYAML != "" && body.DurationMs > 0 {
		var callbackURL string
		if h.callbackURL != "" {
			callbackURL = h.callbackURL + "/" + run.RunID
			log.Printf("Creating run in simulation engine with unique callback URL: %s", callbackURL)
		} else {
			log.Printf("Warning: SIMULATION_CALLBACK_URL not set - simulation engine will not call back when run completes")
		}
		engineRunID, err := h.engineClient.CreateRun(run.RunID, body.ScenarioYAML, body.DurationMs, body.RealTimeMode, callbackURL, h.callbackSecret)
		if err != nil {
			c.JSON(http.StatusCreated, gin.H{
				"run":     run,
				"warning": "run created in backend but failed to create in simulation engine: " + err.Error(),
			})
			return
		}
		run, err = h.simService.UpdateRun(run.RunID, &domain.UpdateRunRequest{EngineRunID: &engineRunID})
		if err != nil {
			c.JSON(http.StatusCreated, gin.H{
				"run":     run,
				"warning": "run created in engine but failed to update engine_run_id in backend",
			})
			return
		}
	}
	c.JSON(http.StatusCreated, gin.H{"run": run})
}

// ListRunsForProject lists runs for a project (project_id in path)
func (h *Handler) ListRunsForProject(c *gin.Context) {
	projectID := c.Param("project_id")
	if projectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project_id is required"})
		return
	}
	if c.GetString("firebase_uid") == "" && c.GetHeader("X-User-Id") == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	runIDs, err := h.simService.ListRunsByProject(projectID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list runs"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"runs": runIDs})
}

// CreateRun creates a new simulation run (user-level, no project)
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
		RealTimeMode *bool                  `json:"real_time_mode,omitempty"` // Enable real-time mode
		Metadata     map[string]interface{} `json:"metadata,omitempty"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Create run in backend first (no project association)
	req := &domain.CreateRunRequest{
		UserID:   userID,
		Metadata: body.Metadata,
	}

	run, err := h.simService.CreateRun(req)
	if err != nil {
		log.Printf("Failed to create run in service: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create run", "details": err.Error()})
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
		engineRunID, err := h.engineClient.CreateRun(run.RunID, body.ScenarioYAML, body.DurationMs, body.RealTimeMode, callbackURL, h.callbackSecret)
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

// GetBestCandidate returns best-candidate info for a run, including S3 path and normalized hosts/services.
func (h *Handler) GetBestCandidate(c *gin.Context) {
	runID := c.Param("id")
	if runID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "run ID is required"})
		return
	}

	if h.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not configured for simulation summaries"})
		return
	}

	// Look up S3 path for best candidate scenario
	var s3Path sql.NullString
	err := h.db.QueryRowContext(
		c.Request.Context(),
		`SELECT best_candidate_s3_path FROM simulation_summaries WHERE run_id = $1`,
		runID,
	).Scan(&s3Path)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "best candidate not available for this run"})
		return
	}
	if err != nil {
		log.Printf("Failed to query best_candidate_s3_path for run_id=%s: %v", runID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load best candidate info"})
		return
	}
	if !s3Path.Valid || s3Path.String == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "best candidate not available for this run"})
		return
	}

	if h.s3Client == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "S3 client not configured; best candidate storage is disabled"})
		return
	}

	// Fetch scenario.yaml from S3
	data, err := h.s3Client.GetObject(c.Request.Context(), s3Path.String)
	if err != nil {
		log.Printf("Failed to fetch best candidate scenario from S3 for run_id=%s, key=%s: %v", runID, s3Path.String, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch best candidate scenario from storage"})
		return
	}

	// Minimal scenario struct just for hosts/services extraction
	type scenarioYAML struct {
		Hosts []struct {
			ID    string `yaml:"id"`
			Cores int    `yaml:"cores"`
		} `yaml:"hosts"`
		Services []struct {
			ID       string  `yaml:"id"`
			Replicas int     `yaml:"replicas"`
			CPUCores float64 `yaml:"cpu_cores"`
			MemoryMB float64 `yaml:"memory_mb"`
		} `yaml:"services"`
	}

	var scenario scenarioYAML
	if err := yaml.Unmarshal(data, &scenario); err != nil {
		log.Printf("Failed to parse best candidate scenario YAML for run_id=%s: %v", runID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse best candidate scenario"})
		return
	}

	// Normalize hosts/services for API response
	type bestCandidateHost struct {
		HostID   string  `json:"host_id"`
		CPUCores int     `json:"cpu_cores"`
		MemoryGB float64 `json:"memory_gb"`
	}
	type bestCandidateService struct {
		ServiceID string  `json:"service_id"`
		Replicas  int     `json:"replicas"`
		CPUCores  float64 `json:"cpu_cores"`
		MemoryMB  float64 `json:"memory_mb"`
	}

	hosts := make([]bestCandidateHost, 0, len(scenario.Hosts))
	for _, hst := range scenario.Hosts {
		hosts = append(hosts, bestCandidateHost{
			HostID:   hst.ID,
			CPUCores: hst.Cores,
			// For now, memory_gb is fixed at 16 as per engine defaults; can be extended later.
			MemoryGB: 16,
		})
	}

	services := make([]bestCandidateService, 0, len(scenario.Services))
	for _, svc := range scenario.Services {
		services = append(services, bestCandidateService{
			ServiceID: svc.ID,
			Replicas:  svc.Replicas,
			CPUCores:  svc.CPUCores,
			MemoryMB:  svc.MemoryMB,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"run_id": runID,
		"best_candidate": gin.H{
			"s3_path":  s3Path.String,
			"hosts":    hosts,
			"services": services,
		},
	})
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

		// Start subscribing to simulator's metrics stream and forwarding to frontend
		// Use default interval of 1000ms (can be made configurable)
		go h.StartMetricsStreamProxy(runID, run.EngineRunID, 1000)
		log.Printf("Started metrics stream proxy for run_id=%s, engine_run_id=%s", runID, run.EngineRunID)
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

// UpdateConfiguration updates the configuration (services, workload, policies) for a running simulation.
// It proxies configuration changes to simulation-core PATCH /v1/runs/{run_id}/configuration.
func (h *Handler) UpdateConfiguration(c *gin.Context) {
	runID := c.Param("id")
	if runID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "run ID is required"})
		return
	}

	var body UpdateRunConfigurationRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	// Require at least one change to be specified
	if len(body.Services) == 0 && len(body.Workload) == 0 && body.Policies == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one of services, workload, or policies must be provided"})
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
	if run.EngineRunID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "run has no engine association"})
		return
	}

	// Verify user has access
	userID := c.GetString("firebase_uid")
	if userID == "" {
		userID = c.GetHeader("X-User-Id")
	}
	if userID == "" || run.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if err := h.engineClient.UpdateRunConfiguration(run.EngineRunID, &body); err != nil {
		log.Printf("Failed to update configuration for run_id=%s: %v", runID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to update configuration in simulation engine: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "configuration updated successfully",
		"run_id":  runID,
	})
}

// UpdateWorkload updates the workload rate for a running simulation.
// Proxies to simulation-core PATCH /v1/runs/{run_id}/workload per BACKEND_INTEGRATION.md.
func (h *Handler) UpdateWorkload(c *gin.Context) {
	runID := c.Param("id")
	if runID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "run ID is required"})
		return
	}

	var body struct {
		PatternKey string  `json:"pattern_key"`
		RateRPS    float64 `json:"rate_rps"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}
	if body.PatternKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pattern_key is required"})
		return
	}
	if body.RateRPS <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "rate_rps must be positive"})
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
	if run.EngineRunID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "run has no engine association"})
		return
	}

	// Verify user has access
	userID := c.GetString("firebase_uid")
	if userID == "" {
		userID = c.GetHeader("X-User-Id")
	}
	if userID == "" || run.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if err := h.engineClient.UpdateWorkloadRate(run.EngineRunID, body.PatternKey, body.RateRPS); err != nil {
		log.Printf("Failed to update workload for run_id=%s: %v", runID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to update workload in simulation engine: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "workload updated successfully",
		"run_id":      runID,
		"pattern_key": body.PatternKey,
	})
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
