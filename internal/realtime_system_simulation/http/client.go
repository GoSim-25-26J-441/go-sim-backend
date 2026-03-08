package http

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// SimulationEngineClient handles communication with the simulation engine
type SimulationEngineClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewSimulationEngineClient creates a new simulation engine client
func NewSimulationEngineClient(baseURL string) *SimulationEngineClient {
	return &SimulationEngineClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// CreateRunRequest represents the request to create a run in the simulation engine
type CreateRunRequest struct {
	RunID string    `json:"run_id,omitempty"`
	Input *RunInput `json:"input"`
}

// OptimizationConfig configures an optimization run (batch or online).
type OptimizationConfig struct {
	Objective            string  `json:"objective,omitempty"`
	MaxIterations        int32   `json:"max_iterations,omitempty"`
	StepSize             float64 `json:"step_size,omitempty"`
	EvaluationDurationMs int64   `json:"evaluation_duration_ms,omitempty"`
	Online               bool    `json:"online,omitempty"`
	TargetP95LatencyMs   float64 `json:"target_p95_latency_ms,omitempty"`
	ControlIntervalMs    int64   `json:"control_interval_ms,omitempty"`
	MinHosts             int32   `json:"min_hosts,omitempty"`
	MaxHosts             int32   `json:"max_hosts,omitempty"`
}

// RunInput represents the input for a simulation run.
// It mirrors simulation-core's RunInput message.
type RunInput struct {
	ScenarioYAML   string              `json:"scenario_yaml"`
	ConfigYAML     string              `json:"config_yaml,omitempty"`
	DurationMs     int64               `json:"duration_ms"`
	Seed           int64               `json:"seed,omitempty"`
	RealTimeMode   *bool               `json:"real_time_mode,omitempty"` // Enable real-time mode for faster simulation
	Optimization   *OptimizationConfig `json:"optimization,omitempty"`
	CallbackURL    string              `json:"callback_url,omitempty"`
	CallbackSecret string              `json:"callback_secret,omitempty"` // Secret for simulator to use when calling back
}

// CreateRunResponse represents the response from creating a run
type CreateRunResponse struct {
	Run struct {
		ID        string `json:"id"`
		Status    string `json:"status"`
		CreatedAt int64  `json:"created_at_unix_ms"`
	} `json:"run"`
}

// CreateRunWithInput creates a run in the simulation engine with a full RunInput payload.
func (c *SimulationEngineClient) CreateRunWithInput(runID string, input *RunInput, callbackURL string, callbackSecret string) (string, error) {
	if input == nil {
		return "", fmt.Errorf("run input is required")
	}

	// Attach callback information to the input before sending.
	input.CallbackURL = callbackURL
	input.CallbackSecret = callbackSecret

	reqBody := CreateRunRequest{
		RunID: runID,
		Input: input,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/runs", c.baseURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call simulation engine: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("simulation engine returned status %d: %s", resp.StatusCode, string(body))
	}

	var createResp CreateRunResponse
	if err := json.Unmarshal(body, &createResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return createResp.Run.ID, nil
}

// CreateRun creates a run in the simulation engine using a minimal set of fields.
// It is kept for backwards compatibility and delegates to CreateRunWithInput.
func (c *SimulationEngineClient) CreateRun(runID string, scenarioYAML string, durationMs int64, realTimeMode *bool, callbackURL string, callbackSecret string) (string, error) {
	input := &RunInput{
		ScenarioYAML: scenarioYAML,
		DurationMs:   durationMs,
		RealTimeMode: realTimeMode,
	}
	return c.CreateRunWithInput(runID, input, callbackURL, callbackSecret)
}

// GetRunResponse represents the response from getting a run
type GetRunResponse struct {
	Run struct {
		ID                  string `json:"id"`
		Status              string `json:"status"`
		CreatedAt           int64  `json:"created_at_unix_ms"`
		StartedAt           int64  `json:"started_at_unix_ms"`
		EndedAt             int64  `json:"ended_at_unix_ms"`
		Error               string `json:"error,omitempty"`
		RealDurationMs      int64  `json:"real_duration_ms,omitempty"`
		SimulationDurationMs int64  `json:"simulation_duration_ms,omitempty"`
	} `json:"run"`
}

// OptimizationStep represents a single optimization step in an online optimization run.
// It mirrors the payload of the optimization_step SSE event and the optimization_history entries
// in the simulator's GET /v1/runs/{id} and /v1/runs/{id}/export responses.
type OptimizationStep struct {
	IterationIndex int                    `json:"iteration_index"`
	TargetP95Ms    float64                `json:"target_p95_ms"`
	ScoreP95Ms     float64                `json:"score_p95_ms"`
	Reason         string                 `json:"reason"`
	PreviousConfig map[string]any         `json:"previous_config,omitempty"`
	CurrentConfig  map[string]any         `json:"current_config,omitempty"`
	Extra          map[string]interface{} `json:"-"` // reserved for future extension if needed
}

// ExportRunRun is the run object in the simulator's export response (convertRunToJSON).
// It includes duration fields when the run has ended and/or input had duration_ms.
type ExportRunRun struct {
	RealDurationMs       int64 `json:"real_duration_ms,omitempty"`
	SimulationDurationMs int64 `json:"simulation_duration_ms,omitempty"`
}

// ExportRunResponse represents the export data from the simulator.
// It contains the original input, aggregated metrics, and optional time-series data.
type ExportRunResponse struct {
	Run  *ExportRunRun `json:"run,omitempty"`
	Input struct {
		ScenarioYAML string `json:"scenario_yaml"`
		DurationMs   int64  `json:"duration_ms,omitempty"`
	} `json:"input"`
	Metrics             map[string]any      `json:"metrics,omitempty"`
	OptimizationHistory []OptimizationStep `json:"optimization_history,omitempty"`
	Candidates          []struct {
		ID           string         `json:"id"`
		Spec         map[string]any `json:"spec"`
		Metrics      map[string]any `json:"metrics"`
		SimWorkload  map[string]any `json:"sim_workload"`
		Source       string         `json:"source"`
		ScenarioYAML string         `json:"scenario_yaml,omitempty"`
	} `json:"candidates,omitempty"`
	TimeSeries          []struct {
		Metric string `json:"metric"`
		Points []struct {
			Timestamp string            `json:"timestamp"`
			Value     float64           `json:"value"`
			Labels    map[string]string `json:"labels"`
		} `json:"points"`
	} `json:"time_series,omitempty"`
}

// UpdateRunConfigurationRequest mirrors the payload expected by the simulation-core
// PATCH /v1/runs/{id}/configuration endpoint.
type UpdateRunConfigurationRequest struct {
	Services []struct {
		ID       string   `json:"id"`
		Replicas int      `json:"replicas"`
		CPUCores *float64 `json:"cpu_cores,omitempty"`
		MemoryMB *float64 `json:"memory_mb,omitempty"`
	} `json:"services,omitempty"`
	Workload []struct {
		PatternKey string  `json:"pattern_key"`
		RateRPS    float64 `json:"rate_rps"`
	} `json:"workload,omitempty"`
	Policies *struct {
		Autoscaling *struct {
			Enabled       bool    `json:"enabled"`
			TargetCPUUtil float64 `json:"target_cpu_util"`
			ScaleStep     int     `json:"scale_step"`
		} `json:"autoscaling,omitempty"`
	} `json:"policies,omitempty"`
}

// GetRun retrieves a run from the simulation engine
func (c *SimulationEngineClient) GetRun(runID string) (*GetRunResponse, error) {
	url := fmt.Sprintf("%s/v1/runs/%s", c.baseURL, runID)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to call simulation engine: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("simulation engine returned status %d: %s", resp.StatusCode, string(body))
	}

	var getResp GetRunResponse
	if err := json.Unmarshal(body, &getResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &getResp, nil
}

// ExportRun fetches the export data for a run and returns the scenario_yaml, if present.
func (c *SimulationEngineClient) ExportRun(runID string) (*ExportRunResponse, error) {
	url := fmt.Sprintf("%s/v1/runs/%s/export", c.baseURL, runID)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to call simulation engine export: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read export response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("simulation engine returned status %d for export: %s", resp.StatusCode, string(body))
	}

	var exportResp ExportRunResponse
	if err := json.Unmarshal(body, &exportResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal export response: %w", err)
	}

	return &exportResp, nil
}

// StartRun starts a run in the simulation engine
func (c *SimulationEngineClient) StartRun(runID string) error {
	url := fmt.Sprintf("%s/v1/runs/%s", c.baseURL, runID)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call simulation engine: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("simulation engine returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// UpdateRunConfiguration sends a configuration update to the simulation engine for a running run.
// It proxies to PATCH /v1/runs/{run_id}/configuration as documented in BACKEND_INTEGRATION.md.
func (c *SimulationEngineClient) UpdateRunConfiguration(runID string, cfg *UpdateRunConfigurationRequest) error {
	if cfg == nil {
		return fmt.Errorf("configuration payload is required")
	}

	body, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal configuration: %w", err)
	}

	url := fmt.Sprintf("%s/v1/runs/%s/configuration", c.baseURL, runID)
	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call simulation engine: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("simulation engine returned status %d for configuration update: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// UpdateWorkloadRate updates the request rate for a workload pattern in a running simulation.
// Per BACKEND_INTEGRATION.md: PATCH /v1/runs/{run_id}/workload
func (c *SimulationEngineClient) UpdateWorkloadRate(runID string, patternKey string, rateRPS float64) error {
	reqBody := map[string]interface{}{
		"pattern_key": patternKey,
		"rate_rps":    rateRPS,
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/runs/%s/workload", c.baseURL, runID)
	req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call simulation engine: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("simulation engine returned status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// StopRun stops a run in the simulation engine
func (c *SimulationEngineClient) StopRun(runID string) error {
	url := fmt.Sprintf("%s/v1/runs/%s:stop", c.baseURL, runID)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call simulation engine: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("simulation engine returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// SimulatorSSEEvent represents an SSE event from the simulator
type SimulatorSSEEvent struct {
	EventType string
	Data      []byte
}

// StreamMetrics subscribes to the simulation engine's metrics stream for a run.
// Endpoint: GET /v1/runs/{run_id}/metrics/stream?interval_ms={interval}
// Note: BACKEND_INTEGRATION.md documents "interval" but simulation-core uses "interval_ms".
// Returns a channel that receives SSE events with event type and data.
func (c *SimulationEngineClient) StreamMetrics(runID string, intervalMs int, ctx context.Context) (<-chan SimulatorSSEEvent, error) {
	// Build URL with optional interval_ms parameter (simulation-core expects interval_ms)
	url := fmt.Sprintf("%s/v1/runs/%s/metrics/stream", c.baseURL, runID)
	if intervalMs > 0 {
		url = fmt.Sprintf("%s?interval_ms=%d", url, intervalMs)
	}

	// Create a client without timeout for SSE streams
	streamClient := &http.Client{
		Timeout: 0, // No timeout for long-running streams
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := streamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to metrics stream: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("simulation engine returned status %d for metrics stream: %s", resp.StatusCode, string(body))
	}

	// Verify Content-Type is text/event-stream
	contentType := resp.Header.Get("Content-Type")
	if contentType != "" && contentType != "text/event-stream" && !bytes.Contains([]byte(contentType), []byte("text/event-stream")) {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected Content-Type for metrics stream: %s (expected text/event-stream)", contentType)
	}

	// Create channel for SSE events with event type
	eventChan := make(chan SimulatorSSEEvent, 10)

	// Read SSE stream in goroutine
	go func() {
		defer resp.Body.Close()
		defer close(eventChan)

		scanner := bufio.NewScanner(resp.Body)
		// Increase buffer size for large JSON payloads
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024) // 1MB max buffer

		var currentEventType string = "message" // Default if no event type specified
		var currentEventData []byte
		var hasData bool

		for scanner.Scan() {
			line := scanner.Bytes()

			// SSE format: "event: <type>\ndata: {...}\n\n"
			if len(line) == 0 {
				// Empty line indicates end of event
				if hasData {
					select {
					case eventChan <- SimulatorSSEEvent{
						EventType: currentEventType,
						Data:      currentEventData,
					}:
					case <-ctx.Done():
						return
					}
					currentEventData = nil
					currentEventType = "message"
					hasData = false
				}
				continue
			}

			// Check if line starts with "event:"
			if bytes.HasPrefix(line, []byte("event: ")) {
				currentEventType = string(bytes.TrimSpace(bytes.TrimPrefix(line, []byte("event: "))))
			} else if bytes.HasPrefix(line, []byte("data: ")) {
				// Data line
				data := bytes.TrimPrefix(line, []byte("data: "))
				if len(currentEventData) > 0 {
					// Multi-line data - append with newline
					currentEventData = append(currentEventData, '\n')
				}
				currentEventData = append(currentEventData, data...)
				hasData = true
			} else if bytes.HasPrefix(line, []byte(":")) {
				// Comment line - ignore
				continue
			} else if hasData {
				// Continuation of data (shouldn't happen with proper SSE, but handle it)
				currentEventData = append(currentEventData, '\n')
				currentEventData = append(currentEventData, line...)
			}
		}

		if err := scanner.Err(); err != nil && err != context.Canceled {
			// Log error but don't block
		}
	}()

	return eventChan, nil
}
