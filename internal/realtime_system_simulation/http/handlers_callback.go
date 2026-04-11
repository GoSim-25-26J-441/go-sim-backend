package http

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"math"
	"net/http"
	"time"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/repository"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/scenario"
	"github.com/gin-gonic/gin"
)

// persistenceTimeout is the max time for callback persistence (export fetch + DB + S3).
// Used in a background goroutine so the HTTP response is sent immediately and the engine
// does not time out. Long enough for batch runs with many candidates (multiple exports + S3 + DB).
const persistenceTimeout = 5 * time.Minute

// engineRunCallbackBody is the payload from the simulation engine. The engine does NOT send
// scenario YAML in the callback; it sends only run IDs (best_run_id, top_candidates). The backend
// must call GET /v1/runs/{id}/export for each ID to obtain input.scenario_yaml.
type engineRunCallbackBody struct {
	RunID           string                 `json:"run_id"`
	Status          interface{}            `json:"status"`        // Can be int (enum) or string
	StatusString    string                 `json:"status_string"` // Preferred: string representation
	CreatedAtUnixMs int64                  `json:"created_at_unix_ms"`
	StartedAtUnixMs int64                  `json:"started_at_unix_ms"`
	EndedAtUnixMs   int64                  `json:"ended_at_unix_ms"`
	Metrics         map[string]interface{} `json:"metrics"`
	TimestampUnixMs int64                  `json:"timestamp"`
	Error           string                 `json:"error,omitempty"` // Error message if any
	// Optimization / batch fields (optional; present for optimization runs)
	BestRunID string   `json:"best_run_id,omitempty"`
	BestScore *float64 `json:"best_score,omitempty"` // hill-climb: objective; batch: efficiency only (legacy)
	// Iterations: batch — search evaluations used; hill-climb — step count (see simulation-core adapter).
	Iterations             int32    `json:"iterations,omitempty"`
	OnlineCompletionReason string   `json:"online_completion_reason,omitempty"`
	TopCandidates          []string `json:"top_candidates,omitempty"`
	// FinalConfig is the last optimization step's current_config (online runs)
	FinalConfig map[string]interface{} `json:"final_config,omitempty"`
	// Structured batch outcome (batch optimization)
	BatchRecommendationFeasible *bool           `json:"batch_recommendation_feasible,omitempty"`
	BatchViolationScore         *float64        `json:"batch_violation_score,omitempty"`
	BatchEfficiencyScore        *float64        `json:"batch_efficiency_score,omitempty"`
	BatchRecommendationSummary  *string         `json:"batch_recommendation_summary,omitempty"`
	BatchScoreBreakdown         json.RawMessage `json:"batch_score_breakdown,omitempty"`
}

func applyOptimizationCallbackMetadata(dst map[string]interface{}, body *engineRunCallbackBody) {
	if body.BestRunID != "" {
		dst["best_run_id"] = body.BestRunID
	}
	if body.BestScore != nil {
		dst["best_score"] = *body.BestScore
	}
	if body.Iterations != 0 {
		dst["iterations"] = body.Iterations
	}
	if len(body.TopCandidates) > 0 {
		dst["top_candidates"] = body.TopCandidates
	}
	if body.FinalConfig != nil {
		dst["final_config"] = body.FinalConfig
	}
	if body.BatchRecommendationFeasible != nil {
		dst["batch_recommendation_feasible"] = *body.BatchRecommendationFeasible
	}
	if body.BatchViolationScore != nil {
		dst["batch_violation_score"] = *body.BatchViolationScore
	}
	if body.BatchEfficiencyScore != nil {
		dst["batch_efficiency_score"] = *body.BatchEfficiencyScore
	}
	if body.BatchRecommendationSummary != nil {
		dst["batch_recommendation_summary"] = *body.BatchRecommendationSummary
	}
	if body.OnlineCompletionReason != "" {
		dst["online_completion_reason"] = body.OnlineCompletionReason
	}
	if len(body.BatchScoreBreakdown) > 0 {
		var bd map[string]interface{}
		if err := json.Unmarshal(body.BatchScoreBreakdown, &bd); err == nil && len(bd) > 0 {
			dst["batch_score_breakdown"] = bd
		}
	}
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

	// Debug logging: summarize payload received from simulator callback.
	metricKeys := make([]string, 0, len(body.Metrics))
	for k := range body.Metrics {
		metricKeys = append(metricKeys, k)
	}
	log.Printf(
		"EngineRunCallback (legacy) received for run_id=%s: status=%s backend_status=%s metrics_keys=%v best_run_id=%s iterations=%d top_candidates=%v error=%s",
		run.RunID,
		statusStr,
		backendStatus,
		metricKeys,
		body.BestRunID,
		body.Iterations,
		body.TopCandidates,
		body.Error,
	)

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

	applyOptimizationCallbackMetadata(updateMeta, &body)

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

	// Terminal state: persist metrics/candidates and best scenario in background so we respond 200
	// immediately and avoid engine callback timeout; use a long detached timeout for batch (many exports + S3 + DB).
	if backendStatus != domain.StatusPending && backendStatus != domain.StatusRunning && (h.db != nil || h.s3Client != nil) {
		runCopy := *run
		bestRunID := body.BestRunID
		topCandidates := make([]string, len(body.TopCandidates))
		copy(topCandidates, body.TopCandidates)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), persistenceTimeout)
			defer cancel()
			h.persistRunMetrics(ctx, &runCopy, bestRunID, topCandidates)
			isBatch := bestRunID != "" || len(topCandidates) > 0
			if h.s3Client != nil && !isBatch {
				h.uploadBestScenarioToS3(ctx, &runCopy, backendStatus, bestRunID)
			}
		}()
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

	// Debug logging: summarize payload received from simulator callback (by ID).
	metricKeys := make([]string, 0, len(body.Metrics))
	for k := range body.Metrics {
		metricKeys = append(metricKeys, k)
	}
	log.Printf(
		"EngineRunCallbackByID received for run_id=%s: status=%s backend_status=%s metrics_keys=%v best_run_id=%s iterations=%d top_candidates=%v error=%s",
		run.RunID,
		statusStr,
		backendStatus,
		metricKeys,
		body.BestRunID,
		body.Iterations,
		body.TopCandidates,
		body.Error,
	)

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

	applyOptimizationCallbackMetadata(updateMeta, &body)

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

	// Terminal state: persist in background so we respond 200 immediately and avoid engine callback timeout.
	if backendStatus != domain.StatusPending && backendStatus != domain.StatusRunning && (h.db != nil || h.s3Client != nil) {
		runCopy := *run
		bestRunID := body.BestRunID
		topCandidates := make([]string, len(body.TopCandidates))
		copy(topCandidates, body.TopCandidates)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), persistenceTimeout)
			defer cancel()
			h.persistRunMetrics(ctx, &runCopy, bestRunID, topCandidates)
			isBatch := bestRunID != "" || len(topCandidates) > 0
			if h.s3Client != nil && !isBatch {
				h.uploadBestScenarioToS3(ctx, &runCopy, backendStatus, bestRunID)
			}
		}()
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
	case 6:
		// Assuming 6 represents STOPPED in the simulator enum.
		return "RUN_STATUS_STOPPED"
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
	case "RUN_STATUS_STOPPED":
		return domain.StatusStopped
	default:
		return domain.StatusPending
	}
}

// firstFloatFromKeys returns the first value from m that is numeric for the given keys (order matters).
// Used to normalize engine metric names (e.g. cpu_utilization -> cpu_util_pct) for the frontend.
func firstFloatFromKeys(m map[string]any, keys ...string) *float64 {
	if m == nil {
		return nil
	}
	for _, k := range keys {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		switch x := v.(type) {
		case float64:
			return &x
		case int:
			f := float64(x)
			return &f
		case int32:
			f := float64(x)
			return &f
		case int64:
			f := float64(x)
			return &f
		}
	}
	return nil
}

// firstFloatFromServiceMetrics returns the first numeric value for the given keys from the first
// element of metrics["service_metrics"], when the engine sends utilisation only inside that slice.
func firstFloatFromServiceMetrics(metrics map[string]any, keys ...string) *float64 {
	if metrics == nil {
		return nil
	}
	v, ok := metrics["service_metrics"]
	if !ok || v == nil {
		return nil
	}
	sl, ok := v.([]interface{})
	if !ok || len(sl) == 0 {
		return nil
	}
	first, ok := sl[0].(map[string]any)
	if !ok {
		return nil
	}
	return firstFloatFromKeys(first, keys...)
}

// floatToCeilInt returns the ceiling of v as an int if v is numeric; otherwise 0.
func floatToCeilInt(v any) int {
	if v == nil {
		return 0
	}
	switch x := v.(type) {
	case float64:
		return int(math.Ceil(x))
	case int:
		return x
	case int32:
		return int(x)
	case int64:
		return int(x)
	default:
		return 0
	}
}

// normalizeSimWorkloadForStorage ensures concurrent_users and rate_rps are stored as integers (ceiling)
// so consumers that expect int (e.g. analysis_suggestions Workload.ConcurrentUsers) can unmarshal.
func normalizeSimWorkloadForStorage(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	if v, ok := m["concurrent_users"]; ok && v != nil {
		out["concurrent_users"] = floatToCeilInt(v)
	}
	if v, ok := m["rate_rps"]; ok && v != nil {
		out["rate_rps"] = floatToCeilInt(v)
	}
	return out
}

// buildSpecMetricsWorkloadFromScenarioAndMetrics builds spec, metrics, and simWorkload
// from a scenario YAML string and metrics map (e.g. from an export response). Used by
// both the single-export fallback and the batch per-candidate export path.
// CPU and memory utilisation are normalised to 0-100 percentage.
// sim_workload.concurrent_users uses the scenario's workload rate_rps (intended load) when present,
// otherwise falls back to metrics throughput_rps (achieved throughput).
// concurrent_users and rate_rps are stored as integers (ceiling) for compatibility with int-typed consumers.
func buildSpecMetricsWorkloadFromScenarioAndMetrics(scenarioYAML string, metrics map[string]any) (spec map[string]any, metricsOut map[string]any, simWorkload map[string]any) {
	spec = map[string]any{"label": "scenario"}
	var parsed scenario.Scenario
	if scenarioYAML != "" {
		s, err := scenario.ParseScenarioYAML([]byte(scenarioYAML))
		if err == nil {
			parsed = s
			if vcpu := s.VCPU(); vcpu > 0 {
				spec["vcpu"] = vcpu
			}
			if memoryGB := s.MemoryGB(); memoryGB > 0 {
				spec["memory_gb"] = memoryGB
			}
		}
	}
	metricsOut = map[string]any{}
	for k, v := range metrics {
		metricsOut[k] = v
	}
	if v := firstFloatFromKeys(metrics, "cpu_util_pct", "cpu_utilization", "cpu_util"); v != nil {
		metricsOut["cpu_util_pct"] = scenario.ToUtilisationPercent(*v)
	} else if v := firstFloatFromServiceMetrics(metrics, "cpu_utilization", "cpu_util"); v != nil {
		metricsOut["cpu_util_pct"] = scenario.ToUtilisationPercent(*v)
	}
	if v := firstFloatFromKeys(metrics, "mem_util_pct", "memory_util_pct", "memory_utilization", "mem_util"); v != nil {
		metricsOut["mem_util_pct"] = scenario.ToUtilisationPercent(*v)
	} else if v := firstFloatFromServiceMetrics(metrics, "memory_utilization", "memory_util_pct", "mem_util"); v != nil {
		metricsOut["mem_util_pct"] = scenario.ToUtilisationPercent(*v)
	}
	simWorkload = map[string]any{}
	if rateRPS := parsed.RateRPS(); rateRPS > 0 {
		simWorkload["concurrent_users"] = int(math.Ceil(rateRPS))
		simWorkload["rate_rps"] = int(math.Ceil(rateRPS))
	} else if v, ok := metrics["throughput_rps"]; ok {
		simWorkload["concurrent_users"] = floatToCeilInt(v)
		simWorkload["rate_rps"] = floatToCeilInt(v)
	}
	return spec, metricsOut, simWorkload
}

// uniqueCandidateIDs returns bestRunID (if non-empty) first, then each of topCandidates
// deduplicated and in order. Used for batch optimization to decide which run IDs to export.
func uniqueCandidateIDs(bestRunID string, topCandidates []string) []string {
	seen := make(map[string]bool)
	var ids []string
	if bestRunID != "" {
		ids = append(ids, bestRunID)
		seen[bestRunID] = true
	}
	for _, id := range topCandidates {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

// persistRunMetrics fetches export data from the simulator and writes summaries and
// time-series metrics into Postgres. The engine does not send scenario YAML in the callback;
// batch candidates are built by calling GET /v1/runs/{id}/export for each ID in best_run_id
// and top_candidates. For batch mode, best-scenario S3 upload and simulation_summaries update
// are done here to avoid duplicate export of the best run.
func (h *Handler) persistRunMetrics(ctx context.Context, run *domain.SimulationRun, bestRunID string, topCandidates []string) {
	if h.db == nil || run == nil || run.RunID == "" {
		return
	}
	if run.EngineRunID == "" && (bestRunID == "" && len(topCandidates) == 0) {
		return
	}

	// Parent export for run-level metrics, time-series, and optimization history.
	// For normal/online we use it for candidates too; for batch we only use it for summary/TS.
	var exportResp *ExportRunResponse
	var exportErr error
	if run.EngineRunID != "" {
		exportResp, exportErr = h.engineClient.ExportRun(run.EngineRunID)
		if exportErr != nil {
			log.Printf("Failed to export metrics for run_id=%s, engine_run_id=%s: %v", run.RunID, run.EngineRunID, exportErr)
			// For batch we might still persist candidates from per-id exports
			if bestRunID == "" && len(topCandidates) == 0 {
				return
			}
		}
	}

	metricsRepo := repository.NewMetricsRepository(h.db)

	// Persist time-series from GET /v1/runs/{id}/metrics/timeseries (cumulative request_count/request_error_count).
	// Do not use export's time_series, which is raw per-event.
	if run.EngineRunID != "" {
		tsResp, err := h.engineClient.GetRunMetricsTimeSeries(ctx, run.EngineRunID, nil)
		if err != nil {
			log.Printf("Failed to fetch metrics timeseries for run_id=%s, engine_run_id=%s: %v", run.RunID, run.EngineRunID, err)
		} else if len(tsResp.Points) > 0 {
			points := make([]repository.TimeSeriesPoint, 0, len(tsResp.Points))
			for _, p := range tsResp.Points {
				if p.Timestamp == "" {
					continue
				}
				parsedTime, err := time.Parse(time.RFC3339Nano, p.Timestamp)
				if err != nil {
					log.Printf("Failed to parse metrics timestamp for run_id=%s metric=%s: %v", run.RunID, p.Metric, err)
					continue
				}
				labels := p.Labels
				if labels == nil {
					labels = map[string]string{}
				}
				serviceID := labels["service_id"]
				if serviceID == "" {
					if v, ok := labels["service"]; ok {
						serviceID = v
					}
				}
				nodeID := labels["host_id"]
				if nodeID == "" {
					if v, ok := labels["host"]; ok {
						nodeID = v
					} else if v, ok := labels["instance"]; ok {
						nodeID = v
					}
				}
				tags := make(map[string]any, len(labels))
				for k, v := range labels {
					tags[k] = v
				}
				points = append(points, repository.TimeSeriesPoint{
					RunID:       run.RunID,
					Time:        parsedTime,
					TimestampMs: parsedTime.UnixMilli(),
					MetricType:  p.Metric,
					MetricValue: p.Value,
					ServiceID:   serviceID,
					NodeID:      nodeID,
					Tags:        tags,
				})
			}
			if len(points) > 0 {
				if insertErr := metricsRepo.InsertTimeSeries(ctx, points); insertErr != nil {
					log.Printf("Failed to insert timeseries metrics for run_id=%s: %v", run.RunID, insertErr)
				}
			}
		}
	}

	if exportResp != nil {
		// Prefer GET /metrics for aggregate; fall back to export's metrics on 412 or error.
		metricsForSummary := exportResp.Metrics
		if run.EngineRunID != "" {
			aggResp, err := h.engineClient.GetRunMetrics(run.EngineRunID)
			if err == nil && aggResp != nil && aggResp.Metrics != nil {
				metricsForSummary = aggResp.Metrics
			} else if err != nil && !errors.Is(err, ErrMetricsNotAvailable) {
				log.Printf("GetRunMetrics for run_id=%s engine_run_id=%s: %v (using export metrics)", run.RunID, run.EngineRunID, err)
			}
		}
		if metricsForSummary != nil {
			var totalDurationMs *int64
			if exportResp.Run != nil && exportResp.Run.SimulationDurationMs > 0 {
				totalDurationMs = &exportResp.Run.SimulationDurationMs
			} else if exportResp.Run != nil && exportResp.Run.RealDurationMs > 0 {
				totalDurationMs = &exportResp.Run.RealDurationMs
			} else if exportResp.Input.DurationMs > 0 {
				totalDurationMs = &exportResp.Input.DurationMs
			}
			summaryData := map[string]any{}
			if exportResp.Run != nil {
				if exportResp.Run.RealDurationMs > 0 {
					summaryData["real_duration_ms"] = exportResp.Run.RealDurationMs
				}
				if exportResp.Run.SimulationDurationMs > 0 {
					summaryData["simulation_duration_ms"] = exportResp.Run.SimulationDurationMs
				}
			}
			summaryParams := &repository.SummaryUpsertParams{
				RunID:           run.RunID,
				EngineRunID:     run.EngineRunID,
				Metrics:         metricsForSummary,
				ScenarioYAML:    exportResp.Input.ScenarioYAML,
				SummaryData:     summaryData,
				TotalDurationMs: totalDurationMs,
			}
			if err := metricsRepo.UpsertSummary(ctx, summaryParams); err != nil {
				log.Printf("Failed to upsert simulation_summaries for run_id=%s: %v", run.RunID, err)
			}
		}

		// Persist optimization history and final_config from export
		if len(exportResp.OptimizationHistory) > 0 || exportResp.FinalConfig != nil {
			meta := make(map[string]interface{})
			if len(exportResp.OptimizationHistory) > 0 {
				meta["optimization_history"] = exportResp.OptimizationHistory
			}
			if exportResp.FinalConfig != nil {
				meta["final_config"] = exportResp.FinalConfig
			}
			_, _ = h.simService.UpdateRun(run.RunID, &domain.UpdateRunRequest{Metadata: meta})
		}
	}

	// Ensure simulation_runs row and candidate repo for candidate persistence
	if _, err := h.db.ExecContext(
		ctx,
		`INSERT INTO simulation_runs (run_id) VALUES ($1)
         ON CONFLICT (run_id) DO NOTHING`,
		run.RunID,
	); err != nil {
		log.Printf("Failed to ensure simulation_runs row for run_id=%s before candidates insert: %v", run.RunID, err)
		return
	}
	candidateRepo := repository.NewCandidateRepository(h.db)

	isBatch := bestRunID != "" || len(topCandidates) > 0
	if isBatch {
		ids := uniqueCandidateIDs(bestRunID, topCandidates)
		records := make([]*repository.CandidateRecord, 0, len(ids))
		bestScenarioKey := "simulation/" + run.RunID + "/best_scenario.yaml"
		for _, id := range ids {
			exp, err := h.engineClient.ExportRun(id)
			if err != nil {
				log.Printf("Failed to export candidate for run_id=%s candidate_id=%s: %v", run.RunID, id, err)
				continue
			}
			spec, metricsOut, simWorkload := buildSpecMetricsWorkloadFromScenarioAndMetrics(exp.Input.ScenarioYAML, exp.Metrics)
			rec := &repository.CandidateRecord{
				UserID: run.UserID,
				ProjectPublicID: sql.NullString{
					String: run.ProjectPublicID,
					Valid:  run.ProjectPublicID != "",
				},
				RunID:       run.RunID,
				CandidateID: id,
				Spec:        spec,
				Metrics:     metricsOut,
				SimWorkload: simWorkload,
				Source:      "export",
			}
			if id == bestRunID {
				rec.S3Path = bestScenarioKey
				// Upload best scenario to S3 and upsert simulation_summaries here to avoid duplicate ExportRun(bestRunID).
				if h.s3Client != nil && h.db != nil && exp.Input.ScenarioYAML != "" {
					if err := h.s3Client.PutObject(ctx, bestScenarioKey, []byte(exp.Input.ScenarioYAML)); err != nil {
						log.Printf("Failed to upload best_scenario.yaml to S3 for run_id=%s: %v", run.RunID, err)
					} else {
						if _, err := h.db.ExecContext(ctx,
							`INSERT INTO simulation_summaries (run_id, engine_run_id, best_candidate_s3_path, scenario_yaml)
         VALUES ($1, $2, $3, $4)
         ON CONFLICT (run_id) DO UPDATE
         SET best_candidate_s3_path = EXCLUDED.best_candidate_s3_path,
             scenario_yaml = COALESCE(EXCLUDED.scenario_yaml, simulation_summaries.scenario_yaml)`,
							run.RunID, run.EngineRunID, bestScenarioKey, exp.Input.ScenarioYAML,
						); err != nil {
							log.Printf("Failed to upsert best_candidate_s3_path for run_id=%s: %v", run.RunID, err)
						} else {
							log.Printf("Best candidate scenario for run_id=%s stored at S3 key=%s and recorded in simulation_summaries", run.RunID, bestScenarioKey)
						}
					}
				}
			} else if h.s3Client != nil && exp.Input.ScenarioYAML != "" {
				key := "simulation/" + run.RunID + "/candidates/" + id + ".yaml"
				if err := h.s3Client.PutObject(ctx, key, []byte(exp.Input.ScenarioYAML)); err != nil {
					log.Printf("Failed to upload candidate YAML to S3 for run_id=%s candidate_id=%s: %v", run.RunID, id, err)
				} else {
					rec.S3Path = key
				}
			}
			records = append(records, rec)
		}
		if len(records) > 0 {
			if err := candidateRepo.CreateMany(ctx, records); err != nil {
				log.Printf("Failed to persist candidates for run_id=%s: %v", run.RunID, err)
			}
		} else if len(ids) > 0 {
			log.Printf("Warning: no candidates persisted for run_id=%s despite %d candidate IDs (all exports may have failed or returned empty scenario_yaml)", run.RunID, len(ids))
		}
		return
	}

	// Non-batch: use single parent export for candidates
	if exportResp == nil {
		return
	}
	if len(exportResp.Candidates) > 0 {
		records := make([]*repository.CandidateRecord, 0, len(exportResp.Candidates))
		for _, cnd := range exportResp.Candidates {
			rec := &repository.CandidateRecord{
				UserID: run.UserID,
				ProjectPublicID: sql.NullString{
					String: run.ProjectPublicID,
					Valid:  run.ProjectPublicID != "",
				},
				RunID:       run.RunID,
				CandidateID: cnd.ID,
				Spec:        cnd.Spec,
				Metrics:     cnd.Metrics,
				SimWorkload: normalizeSimWorkloadForStorage(cnd.SimWorkload),
				Source:      cnd.Source,
			}

			// If S3 is configured and the candidate includes scenario_yaml, upload per-candidate YAML.
			if h.s3Client != nil && cnd.ScenarioYAML != "" {
				key := "simulation/" + run.RunID + "/candidates/" + cnd.ID + ".yaml"
				if err := h.s3Client.PutObject(ctx, key, []byte(cnd.ScenarioYAML)); err != nil {
					log.Printf("Failed to upload candidate YAML to S3 for run_id=%s candidate_id=%s: %v", run.RunID, cnd.ID, err)
				} else {
					rec.S3Path = key
				}
			}

			records = append(records, rec)
		}

		if err := candidateRepo.CreateMany(ctx, records); err != nil {
			log.Printf("Failed to persist candidates for run_id=%s: %v", run.RunID, err)
		}
	} else {
		// Fallback: synthesize a single candidate from the main scenario + metrics.
		spec, metricsOut, simWorkload := buildSpecMetricsWorkloadFromScenarioAndMetrics(exportResp.Input.ScenarioYAML, exportResp.Metrics)
		s3Path := ""
		if h.s3Client != nil {
			s3Path = "simulation/" + run.RunID + "/best_scenario.yaml"
		}
		rec := &repository.CandidateRecord{
			UserID: run.UserID,
			ProjectPublicID: sql.NullString{
				String: run.ProjectPublicID,
				Valid:  run.ProjectPublicID != "",
			},
			RunID:       run.RunID,
			CandidateID: "scenario",
			Spec:        spec,
			Metrics:     metricsOut,
			SimWorkload: simWorkload,
			Source:      "scenario_yaml",
			S3Path:      s3Path,
		}
		if err := candidateRepo.CreateMany(ctx, []*repository.CandidateRecord{rec}); err != nil {
			log.Printf("Failed to persist fallback candidate for run_id=%s: %v", run.RunID, err)
		}
	}
}

// uploadBestScenarioToS3 fetches the best scenario from the simulator export endpoint and uploads it to S3.
// For batch optimization bestRunID is set and we export that run; otherwise we export run.EngineRunID (normal/online).
// It then stores the S3 path in the simulation_summaries table for the given run.
func (h *Handler) uploadBestScenarioToS3(ctx context.Context, run *domain.SimulationRun, status string, bestRunID string) {
	if h.s3Client == nil || h.db == nil {
		return
	}
	if run == nil || run.RunID == "" {
		return
	}
	exportRunID := run.EngineRunID
	if bestRunID != "" {
		exportRunID = bestRunID
	}
	if exportRunID == "" {
		return
	}

	exportResp, err := h.engineClient.ExportRun(exportRunID)
	if err != nil {
		log.Printf("Failed to export run from simulator for run_id=%s, export_run_id=%s: %v", run.RunID, exportRunID, err)
		return
	}

	scenarioYAML := exportResp.Input.ScenarioYAML
	if scenarioYAML == "" {
		log.Printf("Export for run_id=%s, export_run_id=%s did not contain scenario_yaml", run.RunID, exportRunID)
		return
	}

	key := "simulation/" + run.RunID + "/best_scenario.yaml"
	if err := h.s3Client.PutObject(ctx, key, []byte(scenarioYAML)); err != nil {
		log.Printf("Failed to upload best_scenario.yaml to S3 for run_id=%s: %v", run.RunID, err)
		return
	}

	// Ensure there is a reference row in simulation_runs to satisfy FK; insert if missing.
	if _, err := h.db.ExecContext(
		ctx,
		`INSERT INTO simulation_runs (run_id) VALUES ($1)
         ON CONFLICT (run_id) DO NOTHING`,
		run.RunID,
	); err != nil {
		log.Printf("Failed to ensure simulation_runs row for run_id=%s before best_candidate upsert: %v", run.RunID, err)
		return
	}

	// Persist S3 path and scenario_yaml in simulation_summaries; create row if needed.
	if _, err := h.db.ExecContext(
		ctx,
		`INSERT INTO simulation_summaries (run_id, engine_run_id, best_candidate_s3_path, scenario_yaml)
         VALUES ($1, $2, $3, $4)
         ON CONFLICT (run_id) DO UPDATE
         SET best_candidate_s3_path = EXCLUDED.best_candidate_s3_path,
             scenario_yaml = COALESCE(EXCLUDED.scenario_yaml, simulation_summaries.scenario_yaml)`,
		run.RunID, run.EngineRunID, key, scenarioYAML,
	); err != nil {
		log.Printf("Failed to upsert best_candidate_s3_path/scenario_yaml for run_id=%s into simulation_summaries: %v", run.RunID, err)
		return
	}

	log.Printf("Best candidate scenario for run_id=%s stored at S3 key=%s and recorded in simulation_summaries", run.RunID, key)
}
