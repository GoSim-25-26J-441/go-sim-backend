package http

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	simrepo "github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/repository"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/service"
)

func TestCreateRun_OnlineOptimization_RejectsWhenTargetP95MissingOrZero(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Handler with nil simService is fine: we return 400 before calling it
	h := &Handler{simService: nil}
	router := gin.New()
	router.POST("/runs", h.CreateRun)

	tests := []struct {
		name string
		body map[string]interface{}
	}{
		{"online true, no target_p95_latency_ms", map[string]interface{}{
			"optimization": map[string]interface{}{"online": true},
		}},
		{"online true, target_p95_latency_ms zero", map[string]interface{}{
			"optimization": map[string]interface{}{
				"online":                true,
				"target_p95_latency_ms": 0,
			},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(tt.body)
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodPost, "/runs", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-User-Id", "user-1")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
			var resp map[string]interface{}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			assert.Contains(t, resp["error"], "target_p95_latency_ms > 0")
		})
	}
}

func TestCreateRun_OnlineOptimization_AcceptsValidTargetP95(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Mock engine: accept POST /v1/runs and return 201
	engineServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/runs" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"run":{"id":"engine-123","status":"RUN_STATUS_PENDING","created_at_unix_ms":0}}`))
	}))
	defer engineServer.Close()

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	runRepo := simrepo.NewRunRepository(rdb)
	simSvc := service.NewSimulationService(runRepo)
	engineClient := NewSimulationEngineClient(engineServer.URL)

	h := &Handler{
		simService:   simSvc,
		engineClient: engineClient,
		callbackURL:  "http://localhost/callback",
	}
	router := gin.New()
	router.POST("/runs", h.CreateRun)

	body := map[string]interface{}{
		"scenario_yaml": "hosts: []",
		"duration_ms":   10000,
		"optimization": map[string]interface{}{
			"online":                true,
			"target_p95_latency_ms": 50,
		},
	}
	bodyBytes, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/runs", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Validation passed; we should get 201 (engine accepted) not 400
	assert.NotEqual(t, http.StatusBadRequest, w.Code, "valid online optimization should not be rejected with 400")
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestCreateRunForProject_OnlineOptimization_RejectsWhenTargetP95MissingOrZero(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	runRepo := simrepo.NewRunRepository(rdb)
	simSvc := service.NewSimulationService(runRepo)

	h := &Handler{simService: simSvc}
	router := gin.New()
	router.POST("/projects/:project_id/runs", h.CreateRunForProject)

	body := map[string]interface{}{
		"scenario_yaml": "hosts: []",
		"optimization":  map[string]interface{}{"online": true},
	}
	bodyBytes, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/projects/proj-1/runs", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp["error"], "target_p95_latency_ms > 0")
}

func TestCreateRun_BatchOptimization_RejectsWithOnline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handler{simService: nil}
	router := gin.New()
	router.POST("/runs", h.CreateRun)

	body := map[string]interface{}{
		"scenario_yaml": "hosts: []",
		"duration_ms":   5000,
		"optimization": map[string]interface{}{
			"online": true,
			"batch":  map[string]interface{}{},
		},
	}
	bodyBytes, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/runs", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp["error"], "batch optimization cannot be combined")
}

func TestCreateRun_OptimizationObjective_RejectsInvalid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handler{simService: nil}
	router := gin.New()
	router.POST("/runs", h.CreateRun)

	body := map[string]interface{}{
		"scenario_yaml": "hosts: []",
		"duration_ms":   5000,
		"optimization": map[string]interface{}{
			"objective": "invalid_objective",
			"online":    false,
		},
	}
	bodyBytes, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/runs", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp["error"], "invalid optimization.objective")
	assert.Contains(t, resp["error"], "cpu_utilization")
}

func TestCreateRun_BatchOptimization_DefaultMaxEvaluationsForwarded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var captured []byte
	engineServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		captured, err = io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"run":{"id":"engine-b","status":"RUN_STATUS_PENDING","created_at_unix_ms":0}}`))
	}))
	defer engineServer.Close()

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	runRepo := simrepo.NewRunRepository(rdb)
	simSvc := service.NewSimulationService(runRepo)
	engineClient := NewSimulationEngineClient(engineServer.URL)
	h := &Handler{
		simService:   simSvc,
		engineClient: engineClient,
		callbackURL:  "http://localhost/callback",
	}
	router := gin.New()
	router.POST("/runs", h.CreateRun)

	body := map[string]interface{}{
		"scenario_yaml": "hosts: []",
		"duration_ms":   5000,
		"optimization": map[string]interface{}{
			"online": false,
			"batch":  map[string]interface{}{},
		},
	}
	bodyBytes, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/runs", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var createResp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &createResp))
	warns, ok := createResp["warnings"].([]interface{})
	require.True(t, ok, "expected warnings in create response")
	require.NotEmpty(t, warns)

	var outer map[string]interface{}
	require.NoError(t, json.Unmarshal(captured, &outer))
	input := outer["input"].(map[string]interface{})
	opt := input["optimization"].(map[string]interface{})
	assert.Equal(t, float64(DefaultBatchMaxEvaluations), opt["max_evaluations"])
}

func TestCreateRun_RecommendedConfig_RejectedWithoutBatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	runRepo := simrepo.NewRunRepository(rdb)
	simSvc := service.NewSimulationService(runRepo)
	h := &Handler{simService: simSvc}
	router := gin.New()
	router.POST("/runs", h.CreateRun)

	body := map[string]interface{}{
		"scenario_yaml": "hosts: []",
		"duration_ms":   5000,
		"optimization": map[string]interface{}{
			"objective": ObjectiveRecommendedConfig,
			"online":    false,
		},
	}
	bodyBytes, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/runs", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp["error"], "recommended_config")
	assert.Contains(t, resp["error"], "batch")
}

func TestCreateRun_Batch_RecommendedConfig_StartsAndForwardsP95Placeholder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var captured []byte
	engineServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		captured, err = io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"run":{"id":"engine-rc","status":"RUN_STATUS_PENDING","created_at_unix_ms":0}}`))
	}))
	defer engineServer.Close()

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	runRepo := simrepo.NewRunRepository(rdb)
	simSvc := service.NewSimulationService(runRepo)
	engineClient := NewSimulationEngineClient(engineServer.URL)
	h := &Handler{
		simService:   simSvc,
		engineClient: engineClient,
		callbackURL:  "http://localhost/callback",
	}
	router := gin.New()
	router.POST("/runs", h.CreateRun)

	body := map[string]interface{}{
		"scenario_yaml": "hosts: []",
		"duration_ms":   5000,
		"optimization": map[string]interface{}{
			"objective": ObjectiveRecommendedConfig,
			"online":    false,
			"batch":     map[string]interface{}{},
		},
	}
	bodyBytes, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/runs", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var outer map[string]interface{}
	require.NoError(t, json.Unmarshal(captured, &outer))
	input := outer["input"].(map[string]interface{})
	opt := input["optimization"].(map[string]interface{})
	assert.Equal(t, RecommendedConfigEngineObjective, opt["objective"])
}

func TestCreateRun_OptimizationObjective_AcceptsCpuAndMemoryUtilization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engineServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/runs" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"run":{"id":"engine-123","status":"RUN_STATUS_PENDING","created_at_unix_ms":0}}`))
	}))
	defer engineServer.Close()

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	runRepo := simrepo.NewRunRepository(rdb)
	simSvc := service.NewSimulationService(runRepo)
	engineClient := NewSimulationEngineClient(engineServer.URL)
	h := &Handler{
		simService:   simSvc,
		engineClient: engineClient,
		callbackURL:  "http://localhost/callback",
	}
	router := gin.New()
	router.POST("/runs", h.CreateRun)
	router.GET("/runs/:id", h.GetRun)

	for _, objective := range []string{"cpu_utilization", "memory_utilization"} {
		t.Run(objective, func(t *testing.T) {
			body := map[string]interface{}{
				"scenario_yaml": "hosts: []",
				"duration_ms":   5000,
				"optimization": map[string]interface{}{
					"objective": objective,
					"online":    false,
				},
			}
			bodyBytes, err := json.Marshal(body)
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodPost, "/runs", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-User-Id", "user-1")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			assert.Equal(t, http.StatusCreated, w.Code, "objective %q should be accepted", objective)

			var createResp map[string]interface{}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &createResp))
			runObj, ok := createResp["run"].(map[string]interface{})
			require.True(t, ok)
			runID, _ := runObj["run_id"].(string)
			require.NotEmpty(t, runID)

			// GET run and assert metadata.objective is set
			getReq := httptest.NewRequest(http.MethodGet, "/runs/"+runID, nil)
			getReq.Header.Set("X-User-Id", "user-1")
			getW := httptest.NewRecorder()
			router.ServeHTTP(getW, getReq)
			require.Equal(t, http.StatusOK, getW.Code)
			var getResp map[string]interface{}
			require.NoError(t, json.Unmarshal(getW.Body.Bytes(), &getResp))
			run, ok := getResp["run"].(map[string]interface{})
			require.True(t, ok)
			meta, _ := run["metadata"].(map[string]interface{})
			require.NotNil(t, meta)
			assert.Equal(t, objective, meta["objective"], "run.metadata.objective should be set")
		})
	}
}
