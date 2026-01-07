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
	RunID string      `json:"run_id,omitempty"`
	Input *RunInput   `json:"input"`
}

// RunInput represents the input for a simulation run
type RunInput struct {
	ScenarioYAML    string `json:"scenario_yaml"`
	DurationMs      int64  `json:"duration_ms"`
	CallbackURL     string `json:"callback_url,omitempty"`
	CallbackSecret  string `json:"callback_secret,omitempty"` // Secret for simulator to use when calling back
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
func (c *SimulationEngineClient) CreateRun(runID string, scenarioYAML string, durationMs int64, callbackURL string, callbackSecret string) (string, error) {
	reqBody := CreateRunRequest{
		RunID: runID,
		Input: &RunInput{
			ScenarioYAML:   scenarioYAML,
			DurationMs:     durationMs,
			CallbackURL:    callbackURL,
			CallbackSecret: callbackSecret,
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

