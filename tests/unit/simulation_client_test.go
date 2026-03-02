package unit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	simhttp "github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSimulationEngineClient(t *testing.T) {
	c := simhttp.NewSimulationEngineClient("http://localhost:8080")
	require.NotNil(t, c)
}

func TestSimulationEngineClient_CreateRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/runs", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var body struct {
			RunID string `json:"run_id"`
			Input struct {
				ScenarioYAML string `json:"scenario_yaml"`
				DurationMs   int64  `json:"duration_ms"`
			} `json:"input"`
		}
		err := json.NewDecoder(r.Body).Decode(&body)
		require.NoError(t, err)
		assert.Equal(t, "run-123", body.RunID)
		assert.Equal(t, "hosts: []", body.Input.ScenarioYAML)
		assert.Equal(t, int64(5000), body.Input.DurationMs)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"run": map[string]interface{}{
				"id":                 "run-123",
				"status":             "RUN_STATUS_PENDING",
				"created_at_unix_ms": 1705312200000,
			},
		})
	}))
	defer server.Close()

	client := simhttp.NewSimulationEngineClient(server.URL)
	id, err := client.CreateRun("run-123", "hosts: []", 5000, nil, "", "")
	require.NoError(t, err)
	assert.Equal(t, "run-123", id)
}

func TestSimulationEngineClient_CreateRun_Non201(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "run already exists"})
	}))
	defer server.Close()

	client := simhttp.NewSimulationEngineClient(server.URL)
	_, err := client.CreateRun("run-123", "yaml", 1000, nil, "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "409")
	assert.Contains(t, err.Error(), "already exists")
}

func TestSimulationEngineClient_GetRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v1/runs/run-123", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"run": map[string]interface{}{
				"id":                 "run-123",
				"status":             "RUN_STATUS_RUNNING",
				"created_at_unix_ms": 1705312200000,
				"started_at_unix_ms": 1705312201000,
				"ended_at_unix_ms":   0,
			},
		})
	}))
	defer server.Close()

	client := simhttp.NewSimulationEngineClient(server.URL)
	resp, err := client.GetRun("run-123")
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "run-123", resp.Run.ID)
	assert.Equal(t, "RUN_STATUS_RUNNING", resp.Run.Status)
	assert.Equal(t, int64(1705312201000), resp.Run.StartedAt)
}

func TestSimulationEngineClient_GetRun_404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "run not found"})
	}))
	defer server.Close()

	client := simhttp.NewSimulationEngineClient(server.URL)
	_, err := client.GetRun("missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestSimulationEngineClient_StartRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/runs/run-123", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"run": map[string]interface{}{"id": "run-123", "status": "RUN_STATUS_RUNNING"},
		})
	}))
	defer server.Close()

	client := simhttp.NewSimulationEngineClient(server.URL)
	err := client.StartRun("run-123")
	require.NoError(t, err)
}

func TestSimulationEngineClient_StopRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/runs/run-123:stop", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"run": map[string]interface{}{"id": "run-123", "status": "RUN_STATUS_CANCELLED"},
		})
	}))
	defer server.Close()

	client := simhttp.NewSimulationEngineClient(server.URL)
	err := client.StopRun("run-123")
	require.NoError(t, err)
}

func TestSimulationEngineClient_UpdateWorkloadRate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, "/v1/runs/run-123/workload", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var body struct {
			PatternKey string  `json:"pattern_key"`
			RateRPS    float64 `json:"rate_rps"`
		}
		err := json.NewDecoder(r.Body).Decode(&body)
		require.NoError(t, err)
		assert.Equal(t, "client:svc1:/test", body.PatternKey)
		assert.Equal(t, 50.0, body.RateRPS)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"message":     "workload updated successfully",
			"run_id":      "run-123",
			"pattern_key": body.PatternKey,
		})
	}))
	defer server.Close()

	client := simhttp.NewSimulationEngineClient(server.URL)
	err := client.UpdateWorkloadRate("run-123", "client:svc1:/test", 50.0)
	require.NoError(t, err)
}

func TestSimulationEngineClient_UpdateWorkloadRate_Non200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "run is not running"})
	}))
	defer server.Close()

	client := simhttp.NewSimulationEngineClient(server.URL)
	err := client.UpdateWorkloadRate("run-123", "client:svc1:/test", 50.0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "400")
}

func TestSimulationEngineClient_StreamMetrics_URL(t *testing.T) {
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "text/event-stream", r.Header.Get("Accept"))
		assert.Contains(t, r.URL.RawQuery, "interval_ms=1000")

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		_, _ = w.Write([]byte("event: status_change\ndata: {\"status\":\"RUN_STATUS_RUNNING\"}\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		close(done)
	}))
	defer server.Close()

	client := simhttp.NewSimulationEngineClient(server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	eventChan, err := client.StreamMetrics("run-123", 1000, ctx)
	require.NoError(t, err)
	require.NotNil(t, eventChan)

	ev, ok := <-eventChan
	require.True(t, ok)
	assert.Equal(t, "status_change", ev.EventType)
	assert.Contains(t, string(ev.Data), "RUN_STATUS_RUNNING")

	<-done
}

func TestSimulationEngineClient_StreamMetrics_NoIntervalInURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.URL.RawQuery)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		_, _ = w.Write([]byte("event: complete\ndata: {}\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer server.Close()

	client := simhttp.NewSimulationEngineClient(server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	eventChan, err := client.StreamMetrics("run-123", 0, ctx)
	require.NoError(t, err)
	require.NotNil(t, eventChan)
	ev, ok := <-eventChan
	require.True(t, ok)
	assert.Equal(t, "complete", ev.EventType)
}
