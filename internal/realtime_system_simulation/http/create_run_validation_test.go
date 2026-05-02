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

func TestCreateRun_OptimizationCanonicalFields_PrefersCanonicalAndPersistsMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var captured []byte
	engineServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/scenarios:validate":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"valid":true,"errors":[],"warnings":[],"summary":{"hosts":1,"services":1,"workloads":1}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			var err error
			captured, err = io.ReadAll(r.Body)
			require.NoError(t, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"run":{"id":"engine-optim","status":"RUN_STATUS_PENDING","created_at_unix_ms":0}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer engineServer.Close()

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	runRepo := simrepo.NewRunRepository(rdb)
	simSvc := service.NewSimulationService(runRepo)
	h := &Handler{simService: simSvc, engineClient: NewSimulationEngineClient(engineServer.URL), callbackURL: "http://localhost/callback"}
	router := gin.New()
	router.POST("/runs", h.CreateRun)
	router.GET("/runs/:id", h.GetRun)

	body := map[string]interface{}{
		"scenario_yaml": minimalValidCoreScenarioYAML("svc-canonical"),
		"duration_ms":   5000,
		"optimization": map[string]interface{}{
			"online":                       true,
			"objective":                    "p95_latency_ms",
			"optimization_target_primary":  "p95_latency",
			"target_p95_latency_ms":        100,
			"drain_timeout_ms":             12000,
			"host_drain_timeout_ms":        5000, // canonical wins
			"memory_downsize_headroom_mb":  128,
			"memory_headroom_mb":           64, // canonical wins
			"max_controller_steps":         15,
			"max_online_duration_ms":       300000,
			"max_noop_intervals":           10,
			"lease_ttl_ms":                 45000,
			"scale_down_cooldown_ms":       60000,
			"scale_down_cpu_util_max":      0.4,
			"scale_down_mem_util_max":      0.5,
			"scale_down_host_cpu_util_max": 0.35,
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
	assert.Equal(t, float64(12000), opt["drain_timeout_ms"])
	assert.Equal(t, float64(128), opt["memory_downsize_headroom_mb"])
	_, hasLegacyDrain := opt["host_drain_timeout_ms"]
	_, hasLegacyMemory := opt["memory_headroom_mb"]
	assert.False(t, hasLegacyDrain)
	assert.False(t, hasLegacyMemory)

	var createResp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &createResp))
	runObj := createResp["run"].(map[string]interface{})
	runID := runObj["run_id"].(string)
	getReq := httptest.NewRequest(http.MethodGet, "/runs/"+runID, nil)
	getReq.Header.Set("X-User-Id", "user-1")
	getW := httptest.NewRecorder()
	router.ServeHTTP(getW, getReq)
	require.Equal(t, http.StatusOK, getW.Code)
	var getResp map[string]interface{}
	require.NoError(t, json.Unmarshal(getW.Body.Bytes(), &getResp))
	meta := getResp["run"].(map[string]interface{})["metadata"].(map[string]interface{})
	cfg := meta["optimization_config"].(map[string]interface{})
	assert.Equal(t, "online", cfg["mode"])
	assert.Equal(t, float64(12000), cfg["drain_timeout_ms"])
	assert.Equal(t, float64(128), cfg["memory_downsize_headroom_mb"])
}

func TestCreateRun_OptimizationLegacyAliases_NormalizedToCanonical(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var captured []byte
	engineServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/scenarios:validate":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"valid":true,"errors":[],"warnings":[],"summary":{"hosts":1,"services":1,"workloads":1}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			var err error
			captured, err = io.ReadAll(r.Body)
			require.NoError(t, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"run":{"id":"engine-optim-legacy","status":"RUN_STATUS_PENDING","created_at_unix_ms":0}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer engineServer.Close()

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	runRepo := simrepo.NewRunRepository(rdb)
	simSvc := service.NewSimulationService(runRepo)
	h := &Handler{simService: simSvc, engineClient: NewSimulationEngineClient(engineServer.URL), callbackURL: "http://localhost/callback"}
	router := gin.New()
	router.POST("/runs", h.CreateRun)

	body := map[string]interface{}{
		"scenario_yaml": minimalValidCoreScenarioYAML("svc-legacy"),
		"duration_ms":   5000,
		"optimization": map[string]interface{}{
			"online":                true,
			"target_p95_latency_ms": 100,
			"host_drain_timeout_ms": 9000,
			"memory_headroom_mb":    222,
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
	assert.Equal(t, float64(9000), opt["drain_timeout_ms"])
	assert.Equal(t, float64(222), opt["memory_downsize_headroom_mb"])
	_, hasLegacyDrain := opt["host_drain_timeout_ms"]
	_, hasLegacyMemory := opt["memory_headroom_mb"]
	assert.False(t, hasLegacyDrain)
	assert.False(t, hasLegacyMemory)
}

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

	engineServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/scenarios:validate":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"valid":true,"errors":[],"warnings":[],"summary":{"hosts":1,"services":1,"workloads":1}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"run":{"id":"engine-123","status":"RUN_STATUS_PENDING","created_at_unix_ms":0}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
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
		"scenario_yaml": minimalValidCoreScenarioYAML("svc-online-p95-ok"),
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
		"scenario_yaml": minimalValidCoreScenarioYAML("svc-online-p95"),
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
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/scenarios:validate":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"valid":true,"errors":[],"warnings":[],"summary":{"hosts":1,"services":1,"workloads":1}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			var err error
			captured, err = io.ReadAll(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"run":{"id":"engine-b","status":"RUN_STATUS_PENDING","created_at_unix_ms":0}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
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
		"scenario_yaml": minimalValidCoreScenarioYAML("svc-batch-max-eval"),
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
	batch := opt["batch"].(map[string]interface{})
	assert.Equal(t, float64(1), batch["min_hosts"])
	assert.Equal(t, float64(1), batch["max_hosts"])
	assert.Equal(t, float64(4), batch["min_host_cpu_cores"])
	assert.Equal(t, float64(16), batch["min_host_memory_gb"])
	act := batch["allowed_actions"].([]interface{})
	require.GreaterOrEqual(t, len(act), 2)
	assert.Contains(t, act, "BATCH_SCALING_ACTION_SCALE_REPLICAS")
	assert.Contains(t, act, BatchScalingActionScaleHosts)
	_, hasMode := opt["mode"]
	assert.False(t, hasMode, "engine optimization payload must not include BFF-only mode")
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
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/scenarios:validate":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"valid":true,"errors":[],"warnings":[],"summary":{"hosts":1,"services":1,"workloads":1}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			var err error
			captured, err = io.ReadAll(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"run":{"id":"engine-rc","status":"RUN_STATUS_PENDING","created_at_unix_ms":0}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
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
		"scenario_yaml": minimalValidCoreScenarioYAML("svc-rc-rec"),
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
	batch := opt["batch"].(map[string]interface{})
	assert.Equal(t, float64(4), batch["min_host_cpu_cores"])
	assert.Contains(t, batch["allowed_actions"].([]interface{}), BatchScalingActionScaleHosts)
}

func TestCreateRun_Batch_ModeBatchWithoutBatchKey_FillsFleetAndOmitsMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var captured []byte
	engineServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/scenarios:validate":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"valid":true,"errors":[],"warnings":[],"summary":{"hosts":1,"services":1,"workloads":1}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			var err error
			captured, err = io.ReadAll(r.Body)
			require.NoError(t, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"run":{"id":"engine-mode-batch","status":"RUN_STATUS_PENDING","created_at_unix_ms":0}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer engineServer.Close()

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	runRepo := simrepo.NewRunRepository(rdb)
	simSvc := service.NewSimulationService(runRepo)
	h := &Handler{
		simService:   simSvc,
		engineClient: NewSimulationEngineClient(engineServer.URL),
		callbackURL:  "http://localhost/callback",
	}
	router := gin.New()
	router.POST("/runs", h.CreateRun)

	body := map[string]interface{}{
		"scenario_yaml": minimalValidCoreScenarioYAML("svc-mode-batch"),
		"duration_ms":   5000,
		"optimization": map[string]interface{}{
			"mode":      "batch",
			"objective": "cpu_utilization",
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
	require.Equal(t, http.StatusCreated, w.Code)

	var outer map[string]interface{}
	require.NoError(t, json.Unmarshal(captured, &outer))
	input := outer["input"].(map[string]interface{})
	opt := input["optimization"].(map[string]interface{})
	_, hasMode := opt["mode"]
	assert.False(t, hasMode)
	assert.Equal(t, "cpu_utilization", opt["objective"])
	batch := opt["batch"].(map[string]interface{})
	assert.NotEmpty(t, batch)
	assert.Equal(t, float64(1), batch["max_hosts"])
}

func TestCreateRun_BatchOptimization_CpuObjective_EmptyBatch_ForwardedFleetBounds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var captured []byte
	engineServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/scenarios:validate":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"valid":true,"errors":[],"warnings":[],"summary":{"hosts":1,"services":1,"workloads":1}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			var err error
			captured, err = io.ReadAll(r.Body)
			require.NoError(t, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"run":{"id":"engine-cpu-batch","status":"RUN_STATUS_PENDING","created_at_unix_ms":0}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer engineServer.Close()

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	runRepo := simrepo.NewRunRepository(rdb)
	simSvc := service.NewSimulationService(runRepo)
	h := &Handler{
		simService:   simSvc,
		engineClient: NewSimulationEngineClient(engineServer.URL),
		callbackURL:  "http://localhost/callback",
	}
	router := gin.New()
	router.POST("/runs", h.CreateRun)

	body := map[string]interface{}{
		"scenario_yaml": minimalValidCoreScenarioYAML("svc-cpu-empty-batch"),
		"duration_ms":   5000,
		"optimization": map[string]interface{}{
			"objective": "cpu_utilization",
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
	assert.Equal(t, "cpu_utilization", opt["objective"])
	batch := opt["batch"].(map[string]interface{})
	assert.Equal(t, float64(1), batch["min_hosts"])
	assert.Equal(t, float64(1), batch["max_hosts"])
}

func TestCreateRun_OptimizationObjective_AcceptsCpuAndMemoryUtilization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engineServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/scenarios:validate":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"valid":true,"errors":[],"warnings":[],"summary":{"hosts":1,"services":1,"workloads":1}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"run":{"id":"engine-123","status":"RUN_STATUS_PENDING","created_at_unix_ms":0}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
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
				"scenario_yaml": minimalValidCoreScenarioYAML("svc-obj-" + objective),
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
