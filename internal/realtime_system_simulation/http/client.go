package http

import (
	"bytes"
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

// RunInput represents the input for a simulation run
type RunInput struct {
	ScenarioYAML string `json:"scenario_yaml"`
	DurationMs   int64  `json:"duration_ms"`
}

// CreateRunResponse represents the response from creating a run
type CreateRunResponse struct {
	Run struct {
		ID        string `json:"id"`
		Status    string `json:"status"`
		CreatedAt int64  `json:"created_at_unix_ms"`
	} `json:"run"`
}

// CreateRun creates a run in the simulation engine
func (c *SimulationEngineClient) CreateRun(runID string, scenarioYAML string, durationMs int64) (string, error) {
	reqBody := CreateRunRequest{
		RunID: runID,
		Input: &RunInput{
			ScenarioYAML: scenarioYAML,
			DurationMs:   durationMs,
		},
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

// GetRunResponse represents the response from getting a run
type GetRunResponse struct {
	Run struct {
		ID        string `json:"id"`
		Status    string `json:"status"`
		CreatedAt int64  `json:"created_at_unix_ms"`
		StartedAt int64  `json:"started_at_unix_ms"`
		EndedAt   int64  `json:"ended_at_unix_ms"`
		Error     string `json:"error,omitempty"`
	} `json:"run"`
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

// RunSummaryResponse represents the summary/aggregated statistics response from the simulation engine
type RunSummaryResponse struct {
	Summary struct {
		RunID           string                 `json:"run_id"`
		TotalRequests   int64                  `json:"total_requests,omitempty"`
		TotalErrors     int64                  `json:"total_errors,omitempty"`
		TotalDurationMs int64                  `json:"total_duration_ms,omitempty"`
		Metrics         map[string]interface{} `json:"metrics,omitempty"`      // Aggregated metrics (percentiles, averages, etc.)
		SummaryData     map[string]interface{} `json:"summary_data,omitempty"` // Additional summary information
		CreatedAtUnixMs int64                  `json:"created_at_unix_ms,omitempty"`
		StartedAtUnixMs int64                  `json:"started_at_unix_ms,omitempty"`
		EndedAtUnixMs   int64                  `json:"ended_at_unix_ms,omitempty"`
	} `json:"summary"`
}

// GetRunSummary retrieves aggregated summary statistics for a completed run
// Expected endpoint: GET /v1/runs/{id}/summary
func (c *SimulationEngineClient) GetRunSummary(engineRunID string) (*RunSummaryResponse, error) {
	url := fmt.Sprintf("%s/v1/runs/%s/summary", c.baseURL, engineRunID)
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

	var summaryResp RunSummaryResponse
	if err := json.Unmarshal(body, &summaryResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal summary response: %w", err)
	}

	return &summaryResp, nil
}

// MetricDataPointResponse represents a single metric point from the simulation engine
type MetricDataPointResponse struct {
	TimestampMs int64                  `json:"timestamp_ms"`         // Unix timestamp in milliseconds
	MetricType  string                 `json:"metric_type"`          // e.g., "request_latency_ms", "cpu_utilization"
	MetricValue float64                `json:"metric_value"`         // Numeric value
	ServiceID   string                 `json:"service_id,omitempty"` // Optional service identifier
	NodeID      string                 `json:"node_id,omitempty"`    // Optional node identifier
	Tags        map[string]interface{} `json:"tags,omitempty"`       // Additional metadata
}

// RunMetricsResponse represents the timeseries metrics response from the simulation engine
type RunMetricsResponse struct {
	Metrics []MetricDataPointResponse `json:"metrics"`
	// Alternative structure if metrics are nested
	Data struct {
		Metrics []MetricDataPointResponse `json:"metrics"`
	} `json:"data,omitempty"`
}

// GetRunMetrics retrieves timeseries metrics data for a completed run
// Expected endpoint: GET /v1/runs/{id}/metrics
// Optional query parameters: ?from_timestamp_ms={ts}&to_timestamp_ms={ts}&metric_type={type}
func (c *SimulationEngineClient) GetRunMetrics(engineRunID string) ([]MetricDataPointResponse, error) {
	url := fmt.Sprintf("%s/v1/runs/%s/metrics", c.baseURL, engineRunID)
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

	var metricsResp RunMetricsResponse
	if err := json.Unmarshal(body, &metricsResp); err != nil {
		// Try alternative structure: metrics might be directly in array
		var directMetrics []MetricDataPointResponse
		if err2 := json.Unmarshal(body, &directMetrics); err2 == nil {
			return directMetrics, nil
		}
		return nil, fmt.Errorf("failed to unmarshal metrics response: %w", err)
	}

	// Check if metrics are in nested structure
	if len(metricsResp.Data.Metrics) > 0 {
		return metricsResp.Data.Metrics, nil
	}

	return metricsResp.Metrics, nil
}

// GetRunMetricsWithFilters retrieves timeseries metrics with optional filters
func (c *SimulationEngineClient) GetRunMetricsWithFilters(
	engineRunID string,
	fromTimestampMs *int64,
	toTimestampMs *int64,
	metricType string,
) ([]MetricDataPointResponse, error) {
	url := fmt.Sprintf("%s/v1/runs/%s/metrics", c.baseURL, engineRunID)

	// Build query parameters
	queryParams := make([]string, 0)
	if fromTimestampMs != nil {
		queryParams = append(queryParams, fmt.Sprintf("from_timestamp_ms=%d", *fromTimestampMs))
	}
	if toTimestampMs != nil {
		queryParams = append(queryParams, fmt.Sprintf("to_timestamp_ms=%d", *toTimestampMs))
	}
	if metricType != "" {
		queryParams = append(queryParams, fmt.Sprintf("metric_type=%s", metricType))
	}

	if len(queryParams) > 0 {
		url += "?" + queryParams[0]
		for i := 1; i < len(queryParams); i++ {
			url += "&" + queryParams[i]
		}
	}

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

	var metricsResp RunMetricsResponse
	if err := json.Unmarshal(body, &metricsResp); err != nil {
		// Try alternative structure: metrics might be directly in array
		var directMetrics []MetricDataPointResponse
		if err2 := json.Unmarshal(body, &directMetrics); err2 == nil {
			return directMetrics, nil
		}
		return nil, fmt.Errorf("failed to unmarshal metrics response: %w", err)
	}

	// Check if metrics are in nested structure
	if len(metricsResp.Data.Metrics) > 0 {
		return metricsResp.Data.Metrics, nil
	}

	return metricsResp.Metrics, nil
}
