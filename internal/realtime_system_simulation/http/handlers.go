package http

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/domain"
	simrepo "github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/repository"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/scenario"
	"github.com/gin-gonic/gin"
)

func stringFromTags(tags map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := tags[k]; ok && v != nil {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// enrichPersistedTimeSeriesPoint shapes a persisted DB point for JSON responses (nested /metrics and flat /metrics/timeseries).
func enrichPersistedTimeSeriesPoint(p simrepo.TimeSeriesPoint) gin.H {
	tags := p.Tags
	if tags == nil {
		tags = map[string]any{}
	}
	labels := make(map[string]any, len(tags))
	for k, v := range tags {
		labels[k] = v
	}
	serviceID := p.ServiceID
	if serviceID == "" {
		serviceID = stringFromTags(tags, "service_id", "service")
	}
	nodeID := p.NodeID
	hostID := stringFromTags(tags, "host_id", "host")
	instanceID := stringFromTags(tags, "instance_id", "instance")
	if hostID == "" && nodeID != "" {
		hostID = nodeID
	}
	if nodeID == "" {
		if hostID != "" {
			nodeID = hostID
		} else if instanceID != "" {
			nodeID = instanceID
		}
	}
	return gin.H{
		"time":        p.Time,
		"value":       p.MetricValue,
		"labels":      labels,
		"service_id":  serviceID,
		"node_id":     nodeID,
		"host_id":     hostID,
		"instance_id": instanceID,
		"tags":        tags,
	}
}

func persistedTimeSeriesPointFlat(p simrepo.TimeSeriesPoint) gin.H {
	h := enrichPersistedTimeSeriesPoint(p)
	delete(h, "time")
	h["metric"] = p.MetricType
	h["timestamp"] = p.Time.UTC().Format(time.RFC3339Nano)
	return h
}

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
		DiagramVersionID       string                 `json:"diagram_version_id,omitempty"`
		ScenarioYAML           string                 `json:"scenario_yaml,omitempty"`
		SaveScenario           bool                   `json:"save_scenario,omitempty"`
		OverwriteScenarioCache bool                   `json:"overwrite_scenario_cache,omitempty"`
		DurationMs             int64                  `json:"duration_ms,omitempty"`
		RealTimeMode           *bool                  `json:"real_time_mode,omitempty"`
		ConfigYAML             string                 `json:"config_yaml,omitempty"`
		Seed                   int64                  `json:"seed,omitempty"`
		Optimization           json.RawMessage        `json:"optimization,omitempty"`
		Metadata               map[string]interface{} `json:"metadata,omitempty"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	opt, hasOpt, err := UnmarshalOptimizationConfig(body.Optimization)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid optimization", "details": err.Error()})
		return
	}
	if hasOpt {
		if err := validateBatchOptimizationInput(opt.Batch, opt.Online); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		primary := strings.TrimSpace(opt.OptimizationTargetPrimary)
		if primary == "" {
			primary = "p95_latency"
		}
		if opt.Online && primary == "p95_latency" && opt.TargetP95LatencyMs <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "online optimization with primary target p95_latency requires target_p95_latency_ms > 0"})
			return
		}
		if err := validateOptimizationObjective(opt.Objective, batchOptimizationSet(opt.Batch)); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	resolvedDiagramVersionID := strings.TrimSpace(body.DiagramVersionID)
	if h.scenarioCacheRepo != nil {
		if resolvedDiagramVersionID != "" {
			if err := h.scenarioCacheRepo.VerifyDiagramVersionForProject(c.Request.Context(), userID, projectID, resolvedDiagramVersionID); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid diagram_version_id for project/user"})
				return
			}
		} else {
			dvID, err := h.scenarioCacheRepo.ResolveCurrentDiagramVersionID(c.Request.Context(), userID, projectID)
			if err == nil {
				resolvedDiagramVersionID = dvID
			}
		}
	}

	if body.SaveScenario {
		if resolvedDiagramVersionID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "save_scenario requires diagram_version_id"})
			return
		}
		if h.scenarioCacheRepo == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "scenario cache not configured"})
			return
		}
		if strings.TrimSpace(body.ScenarioYAML) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "save_scenario requires scenario_yaml"})
			return
		}
		if _, err := h.validateScenarioPreflight(c.Request.Context(), body.ScenarioYAML); err != nil {
			h.writeScenarioValidationError(c, err)
			return
		}
		amgYAML, err := h.scenarioCacheRepo.GetDiagramYAMLContent(c.Request.Context(), userID, projectID, resolvedDiagramVersionID)
		var sourceHashPtr *string
		if err == nil {
			sh := simrepo.HashAMGAPDSource(amgYAML)
			sourceHashPtr = &sh
		} else if errors.Is(err, simrepo.ErrDiagramMissingYAML) {
			sourceHashPtr = nil
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load diagram YAML for scenario save"})
			return
		}
		s3Path := h.putScenarioToS3(c.Request.Context(), resolvedDiagramVersionID, body.ScenarioYAML)
		_, err = h.scenarioCacheRepo.UpsertScenarioForDiagramVersion(
			c.Request.Context(),
			resolvedDiagramVersionID,
			body.ScenarioYAML,
			"edited",
			s3Path,
			sourceHashPtr,
			body.OverwriteScenarioCache,
		)
		if err != nil {
			if errors.Is(err, simrepo.ErrScenarioCacheConflict) {
				c.JSON(http.StatusConflict, gin.H{"error": "scenario cache conflict for diagram version; set overwrite_scenario_cache=true to replace saved scenario"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save scenario for diagram version"})
			return
		}
	}

	effectiveScenarioYAML := body.ScenarioYAML
	if resolvedDiagramVersionID != "" && h.scenarioCacheRepo != nil && effectiveScenarioYAML == "" {
		yamlStr, _, _, err := h.resolveScenarioYAMLForDiagramVersion(c.Request.Context(), userID, projectID, resolvedDiagramVersionID)
		if err != nil {
			if h.writeScenarioValidationError(c, err) {
				return
			}
			if errors.Is(err, simrepo.ErrDiagramMissingYAML) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "diagram version has no stored AMG/APD YAML"})
				return
			}
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "failed to resolve scenario for diagram version", "details": err.Error()})
			return
		}
		effectiveScenarioYAML = yamlStr
	}
	if effectiveScenarioYAML == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scenario_yaml is required when no diagram version scenario can be resolved"})
		return
	}
	// Authoritative engine validation before any backend run row or POST /v1/runs.
	if _, err := h.validateScenarioPreflight(c.Request.Context(), effectiveScenarioYAML); err != nil {
		h.writeScenarioValidationError(c, err)
		return
	}

	meta := cloneMetadata(body.Metadata)
	if resolvedDiagramVersionID != "" {
		if meta == nil {
			meta = make(map[string]interface{})
		}
		meta["diagram_version_id"] = resolvedDiagramVersionID
	}
	req := &domain.CreateRunRequest{
		UserID:          userID,
		ProjectPublicID: projectID,
		Metadata:        meta,
	}
	run, err := h.simService.CreateRun(req)
	if err != nil {
		log.Printf("Failed to create run in service: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create run", "details": err.Error()})
		return
	}

	online := hasOpt && opt.Online
	batchOpt := hasOpt && batchOptimizationSet(opt.Batch)
	var optimizationGuards []string

	if effectiveScenarioYAML != "" {
		var callbackURL string
		if h.callbackURL != "" {
			callbackURL = h.callbackURL + "/" + run.RunID
			log.Printf("Creating run in simulation engine with unique callback URL: %s", callbackURL)
		} else {
			log.Printf("Warning: SIMULATION_CALLBACK_URL not set - simulation engine will not call back when run completes")
		}
		optPayload := body.Optimization
		if batchOpt {
			var err error
			optPayload, optimizationGuards, err = applyBatchOptimizationGuards(body.Optimization)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid optimization", "details": err.Error()})
				return
			}
		}
		input := &RunInput{
			ScenarioYAML: effectiveScenarioYAML,
			ConfigYAML:   body.ConfigYAML,
			DurationMs:   body.DurationMs,
			Seed:         body.Seed,
			RealTimeMode: body.RealTimeMode,
			Optimization: optPayload,
		}

		engineRunID, err := h.engineClient.CreateRunWithInput(run.RunID, input, callbackURL, h.callbackSecret)
		if err != nil {
			failedStatus := domain.StatusFailed
			_, _ = h.simService.UpdateRun(run.RunID, &domain.UpdateRunRequest{
				Status: &failedStatus,
				Metadata: map[string]interface{}{
					"engine_error": err.Error(),
				},
			})
			var eng *EngineHTTPError
			if errors.As(err, &eng) {
				c.JSON(HTTPStatusForEngineCreate(eng.StatusCode), gin.H{
					"error":   ExtractEngineErrorMessage(eng.Body),
					"details": string(eng.Body),
				})
				return
			}
			c.JSON(http.StatusBadGateway, gin.H{
				"error":   "failed to create run in simulation engine",
				"details": err.Error(),
			})
			return
		}
		updateReq := &domain.UpdateRunRequest{EngineRunID: &engineRunID}
		if hasOpt && (batchOpt || online || opt.Objective != "") {
			meta := make(map[string]interface{})
			if batchOpt {
				meta["mode"] = "batch"
			} else if online {
				meta["mode"] = "online"
			}
			if opt.Objective != "" {
				meta["objective"] = opt.Objective
			}
			updateReq.Metadata = meta
		}
		run, err = h.simService.UpdateRun(run.RunID, updateReq)
		if err != nil {
			resp := gin.H{
				"run":     run,
				"warning": "run created in engine but failed to update engine_run_id in backend",
			}
			if len(optimizationGuards) > 0 {
				resp["warnings"] = optimizationGuards
			}
			c.JSON(http.StatusCreated, resp)
			return
		}

		if h.db != nil {
			metricsRepo := simrepo.NewMetricsRepository(h.db)
			_ = metricsRepo.UpsertSummary(c.Request.Context(), &simrepo.SummaryUpsertParams{
				RunID:        run.RunID,
				EngineRunID:  engineRunID,
				Metrics:      map[string]any{},
				SummaryData:  map[string]any{},
				ScenarioYAML: effectiveScenarioYAML,
			})
		}
	}
	resp := gin.H{"run": run}
	if resolvedDiagramVersionID != "" {
		resp["diagram_version_id"] = resolvedDiagramVersionID
	}
	sum := sha256.Sum256([]byte(effectiveScenarioYAML))
	resp["scenario_hash"] = hex.EncodeToString(sum[:])
	if len(optimizationGuards) > 0 {
		resp["warnings"] = optimizationGuards
	}
	c.JSON(http.StatusCreated, resp)
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

	runs := make([]*domain.SimulationRun, 0, len(runIDs))
	for _, id := range runIDs {
		run, err := h.simService.GetRun(id)
		if err != nil {
			// If a specific run cannot be loaded (e.g., expired), skip it but continue.
			log.Printf("Warning: failed to load run %s for project %s: %v", id, projectID, err)
			continue
		}
		runs = append(runs, run)
	}

	c.JSON(http.StatusOK, gin.H{"runs": runs})
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
		ConfigYAML   string                 `json:"config_yaml,omitempty"`
		Seed         int64                  `json:"seed,omitempty"`
		Optimization json.RawMessage        `json:"optimization,omitempty"`
		Metadata     map[string]interface{} `json:"metadata,omitempty"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	opt, hasOpt, err := UnmarshalOptimizationConfig(body.Optimization)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid optimization", "details": err.Error()})
		return
	}
	if hasOpt {
		if err := validateBatchOptimizationInput(opt.Batch, opt.Online); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		primary := strings.TrimSpace(opt.OptimizationTargetPrimary)
		if primary == "" {
			primary = "p95_latency"
		}
		if opt.Online && primary == "p95_latency" && opt.TargetP95LatencyMs <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "online optimization with primary target p95_latency requires target_p95_latency_ms > 0"})
			return
		}
		if err := validateOptimizationObjective(opt.Objective, batchOptimizationSet(opt.Batch)); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	if body.ScenarioYAML != "" {
		if _, err := h.validateScenarioPreflight(c.Request.Context(), body.ScenarioYAML); err != nil {
			h.writeScenarioValidationError(c, err)
			return
		}
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

	// If scenario_yaml is provided, create run in simulation engine.
	// For online optimization runs, duration_ms can be zero; the controller keeps the run alive.
	// For batch optimization, duration may be zero when the engine drives evaluation via batch settings.
	online := hasOpt && opt.Online
	batchOpt := hasOpt && batchOptimizationSet(opt.Batch)
	var optimizationGuards []string
	if body.ScenarioYAML != "" {
		// Generate unique callback URL per run (includes run_id in path for identification)
		var callbackURL string
		if h.callbackURL != "" {
			// Append run_id to callback URL path: /callback/{run_id}
			callbackURL = h.callbackURL + "/" + run.RunID
			log.Printf("Creating run in simulation engine with unique callback URL: %s", callbackURL)
		} else {
			log.Printf("Warning: SIMULATION_CALLBACK_URL not set - simulation engine will not call back when run completes")
		}
		optPayload := body.Optimization
		if batchOpt {
			var err error
			optPayload, optimizationGuards, err = applyBatchOptimizationGuards(body.Optimization)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid optimization", "details": err.Error()})
				return
			}
		}
		input := &RunInput{
			ScenarioYAML: body.ScenarioYAML,
			ConfigYAML:   body.ConfigYAML,
			DurationMs:   body.DurationMs,
			Seed:         body.Seed,
			RealTimeMode: body.RealTimeMode,
			Optimization: optPayload,
		}

		engineRunID, err := h.engineClient.CreateRunWithInput(run.RunID, input, callbackURL, h.callbackSecret)
		if err != nil {
			failedStatus := domain.StatusFailed
			_, _ = h.simService.UpdateRun(run.RunID, &domain.UpdateRunRequest{
				Status: &failedStatus,
				Metadata: map[string]interface{}{
					"engine_error": err.Error(),
				},
			})
			var eng *EngineHTTPError
			if errors.As(err, &eng) {
				c.JSON(HTTPStatusForEngineCreate(eng.StatusCode), gin.H{
					"error":   ExtractEngineErrorMessage(eng.Body),
					"details": string(eng.Body),
				})
				return
			}
			c.JSON(http.StatusBadGateway, gin.H{
				"error":   "failed to create run in simulation engine",
				"details": err.Error(),
			})
			return
		}

		// Update run with engine run ID (and metadata.mode for online so frontend can show online panel)
		engineRunIDPtr := &engineRunID
		updateReq := &domain.UpdateRunRequest{EngineRunID: engineRunIDPtr}
		if hasOpt && (batchOpt || online || opt.Objective != "") {
			meta := make(map[string]interface{})
			if batchOpt {
				meta["mode"] = "batch"
			} else if online {
				meta["mode"] = "online"
			}
			if opt.Objective != "" {
				meta["objective"] = opt.Objective
			}
			updateReq.Metadata = meta
		}
		run, err = h.simService.UpdateRun(run.RunID, updateReq)
		if err != nil {
			resp := gin.H{
				"run":     run,
				"warning": "run created in engine but failed to update engine_run_id in backend",
			}
			if len(optimizationGuards) > 0 {
				resp["warnings"] = optimizationGuards
			}
			c.JSON(http.StatusCreated, resp)
			return
		}

		// Persist scenario_yaml now so GET /runs/:id returns it while the run is still running.
		if h.db != nil && body.ScenarioYAML != "" {
			if _, err := h.db.ExecContext(
				c.Request.Context(),
				`INSERT INTO simulation_runs (run_id) VALUES ($1) ON CONFLICT (run_id) DO NOTHING`,
				run.RunID,
			); err != nil {
				log.Printf("Failed to ensure simulation_runs row for run_id=%s: %v", run.RunID, err)
			} else if _, err := h.db.ExecContext(
				c.Request.Context(),
				`INSERT INTO simulation_summaries (run_id, engine_run_id, scenario_yaml)
                 VALUES ($1, $2, $3)
                 ON CONFLICT (run_id) DO UPDATE SET
                   engine_run_id = EXCLUDED.engine_run_id,
                   scenario_yaml = COALESCE(EXCLUDED.scenario_yaml, simulation_summaries.scenario_yaml)`,
				run.RunID, run.EngineRunID, body.ScenarioYAML,
			); err != nil {
				log.Printf("Failed to persist scenario_yaml for run_id=%s: %v", run.RunID, err)
			}
		}
	}

	resp := gin.H{"run": run}
	if len(optimizationGuards) > 0 {
		resp["warnings"] = optimizationGuards
	}
	c.JSON(http.StatusCreated, resp)
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

	// Optionally enrich with stored scenario_yaml from simulation_summaries if DB is configured.
	var scenarioYAML *string
	if h.db != nil {
		var yamlText sql.NullString
		err := h.db.QueryRowContext(
			c.Request.Context(),
			`SELECT scenario_yaml FROM simulation_summaries WHERE run_id = $1`,
			runID,
		).Scan(&yamlText)
		if err == nil && yamlText.Valid && yamlText.String != "" {
			scenarioYAML = &yamlText.String
		}
	}

	// Attach scenario_yaml as a top-level field alongside the run for frontend convenience.
	resp := gin.H{"run": run}
	if scenarioYAML != nil {
		resp["scenario_yaml"] = *scenarioYAML
	}

	c.JSON(http.StatusOK, resp)
}

// GetRunMetrics returns persisted summary metrics and time-series for a run.
// This is intended for charting in the frontend after a run completes.
func (h *Handler) GetRunMetrics(c *gin.Context) {
	runID := c.Param("id")
	if runID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "run ID is required"})
		return
	}

	// Auth: ensure user is authenticated
	userID := c.GetString("firebase_uid")
	if userID == "" {
		userID = c.GetHeader("X-User-Id")
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
			return
		}
	}

	// Load run to verify ownership
	run, err := h.simService.GetRun(runID)
	if err != nil {
		if err == domain.ErrRunNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get run"})
		return
	}
	if run.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if h.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not configured for metrics storage"})
		return
	}

	metricsRepo := simrepo.NewMetricsRepository(h.db)

	// Load summary (aggregated) metrics
	summary, err := metricsRepo.GetSummaryByRunID(c.Request.Context(), runID)
	if err != nil {
		log.Printf("Failed to load summary metrics for run_id=%s: %v", runID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load summary metrics"})
		return
	}

	// Load time-series points
	points, err := metricsRepo.ListTimeSeriesByRunID(c.Request.Context(), runID)
	if err != nil {
		log.Printf("Failed to load metrics timeseries for run_id=%s: %v", runID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load timeseries metrics"})
		return
	}

	seriesMap := make(map[string][]gin.H)
	for _, p := range points {
		seriesMap[p.MetricType] = append(seriesMap[p.MetricType], enrichPersistedTimeSeriesPoint(p))
	}

	timeseries := make([]gin.H, 0, len(seriesMap))
	for metric, pts := range seriesMap {
		timeseries = append(timeseries, gin.H{
			"metric": metric,
			"points": pts,
		})
	}

	summaryResp := gin.H{}
	if summary != nil {
		if summary.Metrics != nil {
			summaryResp["metrics"] = summary.Metrics
		}
		if summary.SummaryData != nil {
			summaryResp["summary_data"] = summary.SummaryData
		}
		fc := summary.FinalConfig
		if fc == nil {
			fc = map[string]any{}
		}
		summaryResp["final_config"] = fc
		if summary.TotalRequests.Valid {
			summaryResp["total_requests"] = summary.TotalRequests.Int64
		}
		if summary.TotalErrors.Valid {
			summaryResp["total_errors"] = summary.TotalErrors.Int64
		}
		if summary.TotalDurationMs.Valid {
			summaryResp["total_duration_ms"] = summary.TotalDurationMs.Int64
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"run_id":     run.RunID,
		"summary":    summaryResp,
		"timeseries": timeseries,
	})
}

// GetRunPersistedMetricsTimeSeries returns flat persisted timeseries points from Postgres (not the engine live API).
func (h *Handler) GetRunPersistedMetricsTimeSeries(c *gin.Context) {
	runID := c.Param("id")
	if runID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "run ID is required"})
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

	run, err := h.simService.GetRun(runID)
	if err != nil {
		if err == domain.ErrRunNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get run"})
		return
	}
	if run.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if h.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not configured for metrics storage"})
		return
	}

	metric := strings.TrimSpace(c.Query("metric"))
	metricsRepo := simrepo.NewMetricsRepository(h.db)
	var points []simrepo.TimeSeriesPoint
	if metric != "" {
		points, err = metricsRepo.ListTimeSeriesByRunIDAndMetric(c.Request.Context(), runID, metric)
	} else {
		points, err = metricsRepo.ListTimeSeriesByRunID(c.Request.Context(), runID)
	}
	if err != nil {
		log.Printf("Failed to load persisted timeseries for run_id=%s: %v", runID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load timeseries metrics"})
		return
	}

	out := make([]gin.H, 0, len(points))
	for _, p := range points {
		out = append(out, persistedTimeSeriesPointFlat(p))
	}

	c.JSON(http.StatusOK, gin.H{
		"run_id": run.RunID,
		"points": out,
	})
}

// parseScenarioYAMLHostsServices parses scenario YAML bytes and returns normalized hosts and services for API response.
// Returns (nil, nil) on parse error or empty content. Uses the shared scenario package.
func parseScenarioYAMLHostsServices(data []byte) (hosts []gin.H, services []gin.H) {
	s, err := scenario.ParseScenarioYAML(data)
	if err != nil {
		return nil, nil
	}
	hostsM, servicesM := s.ToHostsServices()
	for _, m := range hostsM {
		hosts = append(hosts, gin.H(m))
	}
	for _, m := range servicesM {
		services = append(services, gin.H(m))
	}
	return hosts, services
}

// GetRunCandidates returns parsed candidate records for a given simulation run.
// Response includes candidates array plus optional best_candidate_id and best_candidate (s3_path, hosts, services) when available.
func (h *Handler) GetRunCandidates(c *gin.Context) {
	runID := c.Param("id")
	if runID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "run ID is required"})
		return
	}

	// Auth: ensure user is authenticated
	userID := c.GetString("firebase_uid")
	if userID == "" {
		userID = c.GetHeader("X-User-Id")
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
			return
		}
	}

	// Load run to verify ownership and get project_id
	run, err := h.simService.GetRun(runID)
	if err != nil {
		if err == domain.ErrRunNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get run"})
		return
	}
	if run.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if h.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not configured for candidates"})
		return
	}

	candidateRepo := simrepo.NewCandidateRepository(h.db)
	records, err := candidateRepo.ListByRunID(c.Request.Context(), runID)
	if err != nil {
		log.Printf("Failed to list candidates for run_id=%s: %v", runID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load candidates"})
		return
	}

	// Derive simulation summary section. For now, we expose node count as the
	// number of distinct labels across candidates' specs (can be refined later).
	// If no candidates, nodes is 0.
	nodes := len(records)

	type candidateDTO struct {
		ID          string                 `json:"id"`
		Spec        map[string]interface{} `json:"spec"`
		Metrics     map[string]interface{} `json:"metrics"`
		SimWorkload map[string]interface{} `json:"sim_workload"`
		Source      string                 `json:"source"`
		S3Path      string                 `json:"s3_path,omitempty"`
	}

	outCandidates := make([]candidateDTO, 0, len(records))
	for _, rec := range records {
		outCandidates = append(outCandidates, candidateDTO{
			ID:          rec.CandidateID,
			Spec:        rec.Spec,
			Metrics:     rec.Metrics,
			SimWorkload: rec.SimWorkload,
			Source:      rec.Source,
			S3Path:      rec.S3Path,
		})
	}

	// Optional: include best-candidate info (s3_path + parsed hosts/services) from simulation_summaries
	var bestCandidateID string
	var bestCandidateObj interface{}
	var bestPath sql.NullString
	if err := h.db.QueryRowContext(
		c.Request.Context(),
		`SELECT best_candidate_s3_path FROM simulation_summaries WHERE run_id = $1`,
		runID,
	).Scan(&bestPath); err == nil && bestPath.Valid && bestPath.String != "" {
		for _, rec := range records {
			if rec.S3Path == bestPath.String {
				bestCandidateID = rec.CandidateID
				break
			}
		}
		if h.s3Client != nil {
			if data, err := h.s3Client.GetObject(c.Request.Context(), bestPath.String); err == nil {
				hosts, services := parseScenarioYAMLHostsServices(data)
				if hosts != nil || services != nil {
					bestCandidateObj = gin.H{
						"s3_path":  bestPath.String,
						"hosts":    hosts,
						"services": services,
					}
				}
			}
		}
	}

	resp := gin.H{
		"user_id":           run.UserID,
		"project_id":        run.ProjectPublicID,
		"run_id":            run.RunID,
		"simulation":        gin.H{"nodes": nodes},
		"best_candidate_id": bestCandidateID,
		"candidates":        outCandidates,
	}
	if bestCandidateObj != nil {
		resp["best_candidate"] = bestCandidateObj
	}
	c.JSON(http.StatusOK, resp)
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

	// If the client wants to change the run status to a state that requires a live simulator
	// but this run never got an engine_run_id (engine run was not created), surface an error
	// instead of silently marking the run as started/completed/cancelled.
	if body.Status != nil && run.EngineRunID == "" {
		if *body.Status == domain.StatusRunning ||
			*body.Status == domain.StatusCompleted ||
			*body.Status == domain.StatusCancelled ||
			*body.Status == domain.StatusStopped {
			c.JSON(http.StatusConflict, gin.H{
				"error": "simulation engine run not created; cannot change status when simulator is not running",
			})
			return
		}
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

	// If status is being set to "cancelled" or "completed" and we have an engine_run_id, stop the run in the engine.
	// - "cancelled": user aborts the run.
	// - "completed": user stops an online/indefinite run (end successfully).
	// - "stopped": explicit stop status from simulator contract.
	if body.Status != nil && run.EngineRunID != "" {
		if *body.Status == domain.StatusCancelled ||
			*body.Status == domain.StatusCompleted ||
			*body.Status == domain.StatusStopped {
			err := h.engineClient.StopRun(run.EngineRunID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": "failed to stop run in simulation engine: " + err.Error(),
				})
				return
			}
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

	// Trigger persistence when run completes successfully
	if body.Status != nil && *body.Status == domain.StatusCompleted && run.EngineRunID != "" {
		go func() {
			ctx := context.Background()
			if err := h.simService.StoreRunSummaryAndMetrics(ctx, runID); err != nil {
				log.Printf("Failed to persist summary and metrics for run_id=%s: %v", runID, err)
			} else {
				log.Printf("Successfully persisted summary and metrics for run_id=%s", runID)
			}
		}()
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

	runs := make([]*domain.SimulationRun, 0, len(runIDs))
	for _, id := range runIDs {
		run, err := h.simService.GetRun(id)
		if err != nil {
			// If a specific run cannot be loaded (e.g., expired), skip it but continue.
			log.Printf("Warning: failed to load run %s for user %s: %v", id, userID, err)
			continue
		}
		runs = append(runs, run)
	}

	c.JSON(http.StatusOK, gin.H{"runs": runs})
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

// RenewOnlineLease proxies POST /v1/runs/{id}/online/renew-lease to simulation-core to extend the wall-clock lease for long online runs.
func (h *Handler) RenewOnlineLease(c *gin.Context) {
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
	if run.EngineRunID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "run has no engine association"})
		return
	}
	userID := c.GetString("firebase_uid")
	if userID == "" {
		userID = c.GetHeader("X-User-Id")
	}
	if userID == "" || run.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}
	if err := h.engineClient.RenewOnlineLease(run.EngineRunID); err != nil {
		var eng *EngineHTTPError
		if errors.As(err, &eng) {
			c.JSON(HTTPStatusForEnginePOST(eng.StatusCode), gin.H{
				"error":   ExtractEngineErrorMessage(eng.Body),
				"details": string(eng.Body),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "run_id": runID})
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

// GetRunSummary retrieves the persisted summary for a completed run
func (h *Handler) GetRunSummary(c *gin.Context) {
	runID := c.Param("id")
	if runID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "run ID is required"})
		return
	}

	summary, err := h.simService.GetStoredSummary(runID)
	if err != nil {
		if err == domain.ErrRunNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "summary not found for this run"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get summary"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"summary": summary})
}
