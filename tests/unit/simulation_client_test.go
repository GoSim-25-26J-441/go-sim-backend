package unit

import (
	"context"
	"encoding/json"
	"errors"
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
				"id":                     "run-123",
				"status":                 "RUN_STATUS_RUNNING",
				"created_at_unix_ms":     1705312200000,
				"started_at_unix_ms":    1705312201000,
				"ended_at_unix_ms":      0,
				"real_duration_ms":      5000,
				"simulation_duration_ms": 5000,
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
	assert.Equal(t, int64(5000), resp.Run.RealDurationMs)
	assert.Equal(t, int64(5000), resp.Run.SimulationDurationMs)
}

func TestSimulationEngineClient_ExportRun_WithRunDurations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v1/runs/run-123/export", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"run": map[string]interface{}{
				"real_duration_ms":       5000,
				"simulation_duration_ms": 5000,
			},
			"input": map[string]interface{}{
				"scenario_yaml": "hosts: []",
				"duration_ms":   5000,
			},
			"metrics": map[string]interface{}{"total_requests": float64(100)},
		})
	}))
	defer server.Close()

	client := simhttp.NewSimulationEngineClient(server.URL)
	resp, err := client.ExportRun("run-123")
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Run, "export run object should be set")
	assert.Equal(t, int64(5000), resp.Run.RealDurationMs)
	assert.Equal(t, int64(5000), resp.Run.SimulationDurationMs)
	assert.Equal(t, "hosts: []", resp.Input.ScenarioYAML)
	assert.Equal(t, int64(5000), resp.Input.DurationMs)
}

func TestSimulationEngineClient_CreateRunWithInput_BatchOptimizationConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		var body struct {
			Input struct {
				Optimization *struct {
					Online                      bool    `json:"online"`
					Batch                       *struct {
						EnableLocalRefinement       *bool   `json:"enable_local_refinement,omitempty"`
						DeterministicCandidateSeeds []int64 `json:"deterministic_candidate_seeds,omitempty"`
					} `json:"batch,omitempty"`
				} `json:"optimization"`
			} `json:"input"`
		}
		err := json.NewDecoder(r.Body).Decode(&body)
		require.NoError(t, err)
		require.NotNil(t, body.Input.Optimization)
		require.NotNil(t, body.Input.Optimization.Batch)
		require.NotNil(t, body.Input.Optimization.Batch.EnableLocalRefinement)
		assert.True(t, *body.Input.Optimization.Batch.EnableLocalRefinement)
		assert.Equal(t, []int64{7, 11}, body.Input.Optimization.Batch.DeterministicCandidateSeeds)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"run": map[string]interface{}{
				"id":                 "run-batch",
				"status":             "RUN_STATUS_PENDING",
				"created_at_unix_ms": 1705312200000,
			},
		})
	}))
	defer server.Close()

	batchInner, err := json.Marshal(map[string]interface{}{
		"enable_local_refinement":       true,
		"deterministic_candidate_seeds": []int64{7, 11},
	})
	require.NoError(t, err)
	opt := simhttp.OptimizationConfig{Online: false, Batch: batchInner}
	optRaw, err := json.Marshal(&opt)
	require.NoError(t, err)

	client := simhttp.NewSimulationEngineClient(server.URL)
	input := &simhttp.RunInput{
		ScenarioYAML: "hosts: []",
		DurationMs:   5000,
		Optimization: optRaw,
	}
	id, err := client.CreateRunWithInput("run-batch", input, "", "")
	require.NoError(t, err)
	assert.Equal(t, "run-batch", id)
}

func TestSimulationEngineClient_CreateRunWithInput_OptimizationConfigWithNewFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/runs", r.URL.Path)
		var body struct {
			Input struct {
				Optimization *struct {
					ScaleDownCPUUtilMax       float64 `json:"scale_down_cpu_util_max"`
					ScaleDownMemUtilMax       float64 `json:"scale_down_mem_util_max"`
					OptimizationTargetPrimary string  `json:"optimization_target_primary"`
					TargetUtilHigh            float64 `json:"target_util_high"`
					TargetUtilLow             float64 `json:"target_util_low"`
					ScaleDownHostCPUUtilMax   float64 `json:"scale_down_host_cpu_util_max"`
				} `json:"optimization"`
			} `json:"input"`
		}
		err := json.NewDecoder(r.Body).Decode(&body)
		require.NoError(t, err)
		require.NotNil(t, body.Input.Optimization)
		assert.Equal(t, 0.5, body.Input.Optimization.ScaleDownCPUUtilMax)
		assert.Equal(t, 0.4, body.Input.Optimization.ScaleDownMemUtilMax)
		assert.Equal(t, "cpu_utilization", body.Input.Optimization.OptimizationTargetPrimary)
		assert.Equal(t, 0.7, body.Input.Optimization.TargetUtilHigh)
		assert.Equal(t, 0.4, body.Input.Optimization.TargetUtilLow)
		assert.Equal(t, 0.3, body.Input.Optimization.ScaleDownHostCPUUtilMax)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"run": map[string]interface{}{
				"id":                 "run-online",
				"status":             "RUN_STATUS_PENDING",
				"created_at_unix_ms": 1705312200000,
			},
		})
	}))
	defer server.Close()

	opt := simhttp.OptimizationConfig{
		Online:                    true,
		TargetP95LatencyMs:        100,
		ScaleDownCPUUtilMax:       0.5,
		ScaleDownMemUtilMax:       0.4,
		OptimizationTargetPrimary: "cpu_utilization",
		TargetUtilHigh:            0.7,
		TargetUtilLow:             0.4,
		ScaleDownHostCPUUtilMax:   0.3,
	}
	optRaw, err := json.Marshal(&opt)
	require.NoError(t, err)

	client := simhttp.NewSimulationEngineClient(server.URL)
	input := &simhttp.RunInput{
		ScenarioYAML: "hosts: []",
		DurationMs:   0,
		Optimization: optRaw,
	}
	id, err := client.CreateRunWithInput("run-online", input, "", "")
	require.NoError(t, err)
	assert.Equal(t, "run-online", id)
}

func TestSimulationEngineClient_ExportRun_WithFinalConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v1/runs/run-final/export", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"input":  map[string]interface{}{"scenario_yaml": "", "duration_ms": 1000},
			"final_config": map[string]interface{}{
				"services": []interface{}{map[string]interface{}{"id": "svc1", "replicas": 2}},
				"hosts":    []interface{}{},
			},
		})
	}))
	defer server.Close()

	client := simhttp.NewSimulationEngineClient(server.URL)
	resp, err := client.ExportRun("run-final")
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.FinalConfig)
	services, ok := resp.FinalConfig["services"].([]interface{})
	require.True(t, ok)
	require.Len(t, services, 1)
	svc, ok := services[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "svc1", svc["id"])
	assert.Equal(t, float64(2), svc["replicas"])
}

func TestSimulationEngineClient_GetRunMetricsTimeSeries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v1/runs/run-123/metrics/timeseries", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"run_id": "run-123",
			"points": []map[string]interface{}{
				{
					"timestamp": "2024-01-15T10:00:00.123456789Z",
					"metric":    "request_count",
					"value":     42.0,
					"labels":   map[string]string{"service": "svc1", "instance": "host-1"},
				},
			},
		})
	}))
	defer server.Close()

	client := simhttp.NewSimulationEngineClient(server.URL)
	ctx := context.Background()
	resp, err := client.GetRunMetricsTimeSeries(ctx, "run-123", nil)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "run-123", resp.RunID)
	require.Len(t, resp.Points, 1)
	assert.Equal(t, "2024-01-15T10:00:00.123456789Z", resp.Points[0].Timestamp)
	assert.Equal(t, "request_count", resp.Points[0].Metric)
	assert.Equal(t, 42.0, resp.Points[0].Value)
	assert.Equal(t, "svc1", resp.Points[0].Labels["service"])
	assert.Equal(t, "host-1", resp.Points[0].Labels["instance"])
}

func TestSimulationEngineClient_GetRunMetricsTimeSeries_WithQueryParams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v1/runs/run-456/metrics/timeseries", r.URL.Path)
		q := r.URL.Query()
		assert.Equal(t, []string{"request_count", "request_error_count"}, q["metric"])
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"run_id": "run-456",
			"points": []map[string]interface{}{},
		})
	}))
	defer server.Close()

	client := simhttp.NewSimulationEngineClient(server.URL)
	ctx := context.Background()
	opts := &simhttp.TimeSeriesQueryOpts{Metric: []string{"request_count", "request_error_count"}}
	resp, err := client.GetRunMetricsTimeSeries(ctx, "run-456", opts)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "run-456", resp.RunID)
	assert.Empty(t, resp.Points)
}

func TestSimulationEngineClient_GetRunMetrics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v1/runs/run-789/metrics", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"metrics": map[string]interface{}{
				"total_requests":   float64(1000),
				"throughput_rps":   float64(50),
				"latency_p95_ms":   float64(12.5),
				"service_metrics":  []interface{}{},
			},
		})
	}))
	defer server.Close()

	client := simhttp.NewSimulationEngineClient(server.URL)
	resp, err := client.GetRunMetrics("run-789")
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Metrics)
	assert.Equal(t, 1000.0, resp.Metrics["total_requests"])
	assert.Equal(t, 50.0, resp.Metrics["throughput_rps"])
	assert.Equal(t, 12.5, resp.Metrics["latency_p95_ms"])
}

func TestSimulationEngineClient_GetRunMetrics_412(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v1/runs/run-412/metrics", r.URL.Path)
		w.WriteHeader(http.StatusPreconditionFailed)
	}))
	defer server.Close()

	client := simhttp.NewSimulationEngineClient(server.URL)
	resp, err := client.GetRunMetrics("run-412")
	require.Error(t, err)
	require.Nil(t, resp)
	assert.True(t, errors.Is(err, simhttp.ErrMetricsNotAvailable), "expected ErrMetricsNotAvailable")
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
