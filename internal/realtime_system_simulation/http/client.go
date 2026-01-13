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

// RunInput represents the input for a simulation run
type RunInput struct {
	ScenarioYAML   string `json:"scenario_yaml"`
	DurationMs     int64  `json:"duration_ms"`
	RealTimeMode   *bool  `json:"real_time_mode,omitempty"` // Enable real-time mode for faster simulation
	CallbackURL    string `json:"callback_url,omitempty"`
	CallbackSecret string `json:"callback_secret,omitempty"` // Secret for simulator to use when calling back
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
func (c *SimulationEngineClient) CreateRun(runID string, scenarioYAML string, durationMs int64, realTimeMode *bool, callbackURL string, callbackSecret string) (string, error) {
	reqBody := CreateRunRequest{
		RunID: runID,
		Input: &RunInput{
			ScenarioYAML:   scenarioYAML,
			DurationMs:     durationMs,
			RealTimeMode:   realTimeMode,
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

// SimulatorSSEEvent represents an SSE event from the simulator
type SimulatorSSEEvent struct {
	EventType string
	Data      []byte
}

// StreamMetrics subscribes to the simulation engine's metrics stream for a run
// Endpoint: GET /v1/runs/{run_id}/metrics/stream?interval_ms={interval}
// Returns a channel that receives SSE events with event type and data
func (c *SimulationEngineClient) StreamMetrics(runID string, intervalMs int, ctx context.Context) (<-chan SimulatorSSEEvent, error) {
	// Build URL with optional interval_ms parameter
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
