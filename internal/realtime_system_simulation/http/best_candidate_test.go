package http

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/domain"
	simrepo "github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/repository"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/service"
)

// fakeObjectStorage is a simple in-memory implementation of ObjectStorage for tests.
type fakeObjectStorage struct {
	objects map[string][]byte
}

func (f *fakeObjectStorage) PutObject(_ context.Context, key string, data []byte) error {
	if f.objects == nil {
		f.objects = make(map[string][]byte)
	}
	f.objects[key] = data
	return nil
}

func (f *fakeObjectStorage) GetObject(_ context.Context, key string) ([]byte, error) {
	if f.objects == nil {
		return nil, nil
	}
	return f.objects[key], nil
}

// TestHandler_GetRunCandidates_Unified tests the unified candidates endpoint returns
// candidates array plus best_candidate_id and best_candidate when summary and S3 are available.
func TestHandler_GetRunCandidates_Unified(t *testing.T) {
	gin.SetMode(gin.TestMode)

	runID := "run-123"
	userID := "user-456"
	s3Key := "simulation/run-123/best_scenario.yaml"
	bestCandidateID := "best-1"

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	run := &domain.SimulationRun{
		RunID:           runID,
		UserID:          userID,
		ProjectPublicID: "proj-1",
		Status:          domain.StatusCompleted,
		EngineRunID:     "engine-123",
	}
	runData, err := json.Marshal(run)
	require.NoError(t, err)
	err = rdb.Set(context.Background(), "sim:run:"+runID, runData, 0).Err()
	require.NoError(t, err)

	runRepo := simrepo.NewRunRepository(rdb)
	simSvc := service.NewSimulationService(runRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Candidates list query (ListByRunID); nodes come from spec.hosts (2 hosts), not candidate count.
	specJSON := []byte(`{"hosts":[{"id":"host-1"},{"id":"host-2"}]}`)
	mock.ExpectQuery(`SELECT id, user_id, project_public_id, run_id, candidate_id`).
		WithArgs(driver.Value(runID)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "project_public_id", "run_id", "candidate_id",
			"spec", "metrics", "sim_workload", "source", "s3_path",
		}).AddRow(
			"uuid-1", userID, "proj-1", runID, bestCandidateID,
			specJSON, []byte("{}"), []byte("{}"), "export", s3Key,
		))

	// Best candidate path query
	mock.ExpectQuery(`SELECT best_candidate_s3_path FROM simulation_summaries WHERE run_id = \$1`).
		WithArgs(driver.Value(runID)).
		WillReturnRows(sqlmock.NewRows([]string{"best_candidate_s3_path"}).AddRow(s3Key))

	yamlContent := `
hosts:
  - id: host-1
    cores: 4
  - id: host-2
    cores: 4
services:
  - id: svc1
    replicas: 3
    cpu_cores: 1
    memory_mb: 512
`
	storage := &fakeObjectStorage{
		objects: map[string][]byte{s3Key: []byte(yamlContent)},
	}

	h := &Handler{
		simService: simSvc,
		db:         db,
		s3Client:   storage,
	}

	router := gin.New()
	router.GET("/runs/:id/candidates", h.GetRunCandidates)

	req := httptest.NewRequest(http.MethodGet, "/runs/"+runID+"/candidates", nil)
	req.Header.Set("X-User-Id", userID)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var resp struct {
		UserID          string          `json:"user_id"`
		ProjectID       string          `json:"project_id"`
		RunID           string          `json:"run_id"`
		Simulation      struct {
			Nodes int `json:"nodes"`
		} `json:"simulation"`
		BestCandidateID string          `json:"best_candidate_id"`
		BestCandidate   json.RawMessage `json:"best_candidate"`
		Candidates      []struct {
			ID   string                 `json:"id"`
			Spec map[string]interface{} `json:"spec"`
		} `json:"candidates"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, userID, resp.UserID)
	assert.Equal(t, runID, resp.RunID)
	assert.Equal(t, 2, resp.Simulation.Nodes, "simulation.nodes should count spec.hosts, not candidates")
	assert.Equal(t, bestCandidateID, resp.BestCandidateID)
	require.Len(t, resp.Candidates, 1)
	assert.Equal(t, bestCandidateID, resp.Candidates[0].ID)

	require.True(t, len(resp.BestCandidate) > 0, "best_candidate should be present")
	var bc struct {
		S3Path string `json:"s3_path"`
		Hosts  []struct {
			HostID   string  `json:"host_id"`
			CPUCores int     `json:"cpu_cores"`
			MemoryGB float64 `json:"memory_gb"`
		} `json:"hosts"`
		Services []struct {
			ServiceID string  `json:"service_id"`
			Replicas  int     `json:"replicas"`
			CPUCores  float64 `json:"cpu_cores"`
			MemoryMB  float64 `json:"memory_mb"`
		} `json:"services"`
	}
	err = json.Unmarshal(resp.BestCandidate, &bc)
	require.NoError(t, err)
	assert.Equal(t, s3Key, bc.S3Path)
	require.Len(t, bc.Hosts, 2)
	assert.Equal(t, "host-1", bc.Hosts[0].HostID)
	assert.Equal(t, 4, bc.Hosts[0].CPUCores)
	require.Len(t, bc.Services, 1)
	assert.Equal(t, "svc1", bc.Services[0].ServiceID)
	assert.Equal(t, 3, bc.Services[0].Replicas)

	require.NoError(t, mock.ExpectationsWereMet())
}

// TestHandler_GetRunCandidates_SimulationNodesUsesBestCandidateHosts verifies simulation.nodes
// comes from the best candidate's spec.hosts length when five candidates exist.
func TestHandler_GetRunCandidates_SimulationNodesUsesBestCandidateHosts(t *testing.T) {
	gin.SetMode(gin.TestMode)

	runID := "run-nodes-1"
	userID := "user-nodes"
	s3Best := "simulation/run-nodes-1/best.yaml"
	specThreeHosts := []byte(`{"hosts":[{"id":"h1"},{"id":"h2"},{"id":"h3"}]}`)
	specEmpty := []byte(`{}`)

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	run := &domain.SimulationRun{
		RunID:           runID,
		UserID:          userID,
		ProjectPublicID: "proj-nodes",
		Status:          domain.StatusCompleted,
		EngineRunID:     "engine-nodes",
	}
	runData, err := json.Marshal(run)
	require.NoError(t, err)
	require.NoError(t, rdb.Set(context.Background(), "sim:run:"+runID, runData, 0).Err())

	runRepo := simrepo.NewRunRepository(rdb)
	simSvc := service.NewSimulationService(runRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	rows := sqlmock.NewRows([]string{
		"id", "user_id", "project_public_id", "run_id", "candidate_id",
		"spec", "metrics", "sim_workload", "source", "s3_path",
	})
	for i := range 5 {
		cid := []string{"cand-0", "cand-1", "cand-2", "cand-3", "cand-4"}[i]
		spec := specEmpty
		path := ""
		if i == 4 {
			spec = specThreeHosts
			path = s3Best
		}
		rows.AddRow(
			"uuid-"+cid, userID, "proj-nodes", runID, cid,
			spec, []byte("{}"), []byte("{}"), "export", path,
		)
	}
	mock.ExpectQuery(`SELECT id, user_id, project_public_id, run_id, candidate_id`).
		WithArgs(driver.Value(runID)).
		WillReturnRows(rows)

	mock.ExpectQuery(`SELECT best_candidate_s3_path FROM simulation_summaries WHERE run_id = \$1`).
		WithArgs(driver.Value(runID)).
		WillReturnRows(sqlmock.NewRows([]string{"best_candidate_s3_path"}).AddRow(s3Best))

	h := &Handler{
		simService: simSvc,
		db:         db,
		s3Client:   &fakeObjectStorage{objects: map[string][]byte{}},
	}

	router := gin.New()
	router.GET("/runs/:id/candidates", h.GetRunCandidates)

	req := httptest.NewRequest(http.MethodGet, "/runs/"+runID+"/candidates", nil)
	req.Header.Set("X-User-Id", userID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var resp struct {
		Simulation struct {
			Nodes int `json:"nodes"`
		} `json:"simulation"`
		BestCandidateID string `json:"best_candidate_id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 3, resp.Simulation.Nodes)
	assert.Equal(t, "cand-4", resp.BestCandidateID)

	require.NoError(t, mock.ExpectationsWereMet())
}

// TestHandler_GetRunCandidates_SimulationNodesZeroWithoutHosts verifies simulation.nodes is 0
// when no candidate spec contains a valid hosts array.
func TestHandler_GetRunCandidates_SimulationNodesZeroWithoutHosts(t *testing.T) {
	gin.SetMode(gin.TestMode)

	runID := "run-no-hosts"
	userID := "user-no-hosts"

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	run := &domain.SimulationRun{
		RunID:           runID,
		UserID:          userID,
		ProjectPublicID: "proj-nh",
		Status:          domain.StatusCompleted,
		EngineRunID:     "engine-nh",
	}
	runData, err := json.Marshal(run)
	require.NoError(t, err)
	require.NoError(t, rdb.Set(context.Background(), "sim:run:"+runID, runData, 0).Err())

	runRepo := simrepo.NewRunRepository(rdb)
	simSvc := service.NewSimulationService(runRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	specEmpty := []byte(`{}`)
	rows := sqlmock.NewRows([]string{
		"id", "user_id", "project_public_id", "run_id", "candidate_id",
		"spec", "metrics", "sim_workload", "source", "s3_path",
	})
	for i := range 5 {
		cid := []string{"cand-0", "cand-1", "cand-2", "cand-3", "cand-4"}[i]
		rows.AddRow(
			"uuid-"+cid, userID, "proj-nh", runID, cid,
			specEmpty, []byte("{}"), []byte("{}"), "export", "",
		)
	}
	mock.ExpectQuery(`SELECT id, user_id, project_public_id, run_id, candidate_id`).
		WithArgs(driver.Value(runID)).
		WillReturnRows(rows)

	mock.ExpectQuery(`SELECT best_candidate_s3_path FROM simulation_summaries WHERE run_id = \$1`).
		WithArgs(driver.Value(runID)).
		WillReturnRows(sqlmock.NewRows([]string{"best_candidate_s3_path"}).AddRow(nil))

	h := &Handler{
		simService: simSvc,
		db:         db,
	}

	router := gin.New()
	router.GET("/runs/:id/candidates", h.GetRunCandidates)

	req := httptest.NewRequest(http.MethodGet, "/runs/"+runID+"/candidates", nil)
	req.Header.Set("X-User-Id", userID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var resp struct {
		Simulation struct {
			Nodes int `json:"nodes"`
		} `json:"simulation"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 0, resp.Simulation.Nodes)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUploadBestScenarioToS3_StoresObjectAndDBRecord(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Mock simulation engine export endpoint
	engineRunID := "engine-123"
	runID := "run-123"

	engineServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v1/runs/"+engineRunID+"/export", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"input": {
				"scenario_yaml": "hosts:\n  - id: host-1\n    cores: 4\n"
			}
		}`))
	}))
	defer engineServer.Close()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	storage := &fakeObjectStorage{objects: make(map[string][]byte)}

	// Expect ensure simulation_runs row, then upsert into simulation_summaries
	mock.ExpectExec(`INSERT INTO simulation_runs`).
		WithArgs(driver.Value(runID)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO simulation_summaries`).
		WithArgs(driver.Value(runID), driver.Value(engineRunID), driver.Value("simulation/"+runID+"/best_scenario.yaml"), driver.Value("hosts:\n  - id: host-1\n    cores: 4\n")).
		WillReturnResult(sqlmock.NewResult(1, 1))

	h := &Handler{
		engineClient: NewSimulationEngineClient(engineServer.URL),
		db:           db,
		s3Client:     storage,
	}

	run := &domain.SimulationRun{
		RunID:       runID,
		EngineRunID: engineRunID,
	}

	h.uploadBestScenarioToS3(context.Background(), run, "completed", "")

	// Ensure object was stored in fake S3
	data, ok := storage.objects["simulation/"+runID+"/best_scenario.yaml"]
	require.True(t, ok)
	assert.Contains(t, string(data), "hosts:")

	require.NoError(t, mock.ExpectationsWereMet())
}

// TestUploadBestScenarioToS3_UsesBestRunIDWhenSet verifies that when bestRunID is set,
// the export is fetched for bestRunID rather than run.EngineRunID (batch optimization path).
func TestUploadBestScenarioToS3_UsesBestRunIDWhenSet(t *testing.T) {
	gin.SetMode(gin.TestMode)

	runID := "run-123"
	engineRunID := "engine-123"
	bestRunID := "best-789"

	var exportPath string
	engineServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		exportPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"input":{"scenario_yaml":"hosts:\n  - id: h1\n    cores: 2\n"}}`))
	}))
	defer engineServer.Close()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`INSERT INTO simulation_runs`).
		WithArgs(driver.Value(runID)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO simulation_summaries`).
		WithArgs(driver.Value(runID), driver.Value(engineRunID), driver.Value("simulation/"+runID+"/best_scenario.yaml"), driver.Value("hosts:\n  - id: h1\n    cores: 2\n")).
		WillReturnResult(sqlmock.NewResult(1, 1))

	h := &Handler{
		engineClient: NewSimulationEngineClient(engineServer.URL),
		db:           db,
		s3Client:     &fakeObjectStorage{objects: make(map[string][]byte)},
	}
	run := &domain.SimulationRun{
		RunID:       runID,
		EngineRunID: engineRunID,
	}

	h.uploadBestScenarioToS3(context.Background(), run, "completed", bestRunID)

	assert.Equal(t, "/v1/runs/"+bestRunID+"/export", exportPath, "export should be called with best_run_id, not engine_run_id")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestUniqueCandidateIDs verifies deduplication and best-first order.
func TestUniqueCandidateIDs(t *testing.T) {
	ids := uniqueCandidateIDs("best-1", []string{"best-1", "cand-2", "cand-2"})
	assert.Equal(t, []string{"best-1", "cand-2"}, ids)

	ids = uniqueCandidateIDs("", []string{"a", "b"})
	assert.Equal(t, []string{"a", "b"}, ids)

	ids = uniqueCandidateIDs("only", nil)
	assert.Equal(t, []string{"only"}, ids)
}

// TestBuildSpecMetricsWorkloadFromScenarioAndMetrics_PercentageNormalization verifies that
// CPU and memory utilisation from the engine (ratio 0-1) are normalised to 0-100 percentage.
func TestBuildSpecMetricsWorkloadFromScenarioAndMetrics_PercentageNormalization(t *testing.T) {
	scenarioYAML := `
hosts:
  - id: h1
    cores: 4
`
	metrics := map[string]any{
		"cpu_utilization":    0.72,
		"memory_utilization": 0.61,
		"throughput_rps":     1400,
	}
	spec, metricsOut, simWorkload := buildSpecMetricsWorkloadFromScenarioAndMetrics(scenarioYAML, metrics)

	require.NotNil(t, metricsOut)
	assert.Equal(t, 72.0, metricsOut["cpu_util_pct"], "ratio 0.72 should become 72%")
	assert.Equal(t, 61.0, metricsOut["mem_util_pct"], "ratio 0.61 should become 61%")
	assert.Equal(t, 4.0, spec["vcpu"])
	assert.Equal(t, 16.0, spec["memory_gb"])
	assert.EqualValues(t, 1400, simWorkload["concurrent_users"])
}

// TestBuildSpecMetricsWorkloadFromScenarioAndMetrics_AlreadyPercentage verifies that
// values already in 0-100 scale are left unchanged (and clamped to 100 if over).
func TestBuildSpecMetricsWorkloadFromScenarioAndMetrics_AlreadyPercentage(t *testing.T) {
	metrics := map[string]any{
		"cpu_util_pct": 72,
		"mem_util_pct": 61,
	}
	_, metricsOut, _ := buildSpecMetricsWorkloadFromScenarioAndMetrics("", metrics)
	assert.Equal(t, 72.0, metricsOut["cpu_util_pct"])
	assert.Equal(t, 61.0, metricsOut["mem_util_pct"])
}

// TestBuildSpecMetricsWorkloadFromScenarioAndMetrics_WorkloadRateRPS verifies that
// when scenario YAML has workload.arrival.rate_rps, sim_workload uses it for concurrent_users
// (intended load) instead of metrics throughput_rps (achieved throughput).
func TestBuildSpecMetricsWorkloadFromScenarioAndMetrics_WorkloadRateRPS(t *testing.T) {
	scenarioYAML := `
hosts:
  - id: host-1
    cores: 4
    memory_gb: 16
workload:
  - from: client
    to: svc1:/test
    arrival:
      type: poisson
      rate_rps: 10
`
	metrics := map[string]any{
		"throughput_rps":  6033.31,
		"cpu_utilization": 0.056,
	}
	spec, _, simWorkload := buildSpecMetricsWorkloadFromScenarioAndMetrics(scenarioYAML, metrics)
	assert.Equal(t, 4.0, spec["vcpu"])
	assert.Equal(t, 16.0, spec["memory_gb"], "memory_gb should come from host memory_gb, not service")
	assert.EqualValues(t, 10, simWorkload["concurrent_users"], "concurrent_users should be scenario rate_rps (10), not throughput_rps")
	assert.EqualValues(t, 10, simWorkload["rate_rps"])
}

// TestBuildSpecMetricsWorkloadFromScenarioAndMetrics_ServiceMetricsFallback verifies that when
// the engine sends utilisation only inside service_metrics (no top-level keys), cpu_util_pct and
// mem_util_pct are set from the first service and normalised to 0-100.
func TestBuildSpecMetricsWorkloadFromScenarioAndMetrics_ServiceMetricsFallback(t *testing.T) {
	metrics := map[string]any{
		"throughput_rps": 100.0,
		"service_metrics": []interface{}{
			map[string]any{
				"cpu_utilization":    0.075,
				"memory_utilization": 0.036,
			},
		},
	}
	_, metricsOut, _ := buildSpecMetricsWorkloadFromScenarioAndMetrics("", metrics)

	require.NotNil(t, metricsOut)
	cpu, ok := metricsOut["cpu_util_pct"]
	require.True(t, ok, "cpu_util_pct should be set from service_metrics")
	assert.InDelta(t, 7.5, cpu, 0.01, "0.075 ratio should become 7.5%%")

	mem, ok := metricsOut["mem_util_pct"]
	require.True(t, ok, "mem_util_pct should be set from service_metrics")
	assert.InDelta(t, 3.6, mem, 0.01, "0.036 ratio should become 3.6%%")
}

func TestPersistRunMetrics_OrdinaryFallbackUsesParentRunIDCandidate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	runID := "run-ordinary-1"
	engineRunID := "engine-ordinary-1"
	userID := "user-ordinary"

	engineServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+engineRunID:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"run":{"id":"engine-ordinary-1","status":"RUN_STATUS_COMPLETED"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+engineRunID+"/export":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
  "input": {
    "scenario_yaml": "hosts:\n  - id: host-1\n    cores: 2\nservices:\n  - id: checkout\n    replicas: 1\n    model: cpu\n    endpoints:\n      - path: /read\n        mean_cpu_ms: 1\n        cpu_sigma_ms: 0\n        net_latency_ms: {mean: 1, sigma: 0.1}\n        downstream: []\nworkload:\n  - from: client\n    to: checkout:/read\n    arrival:\n      type: poisson\n      rate_rps: 10\n",
    "duration_ms": 5000
  },
  "metrics": {
    "throughput_rps": 10,
    "cpu_utilization": 0.2,
    "memory_utilization": 0.1
  }
}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+engineRunID+"/metrics/timeseries":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"run_id":"engine-ordinary-1","points":[]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+engineRunID+"/metrics":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"metrics":{"throughput_rps":10}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer engineServer.Close()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`INSERT INTO simulation_runs`).
		WithArgs(driver.Value(runID)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO simulation_summaries`).
		WithArgs(
			driver.Value(runID),
			driver.Value(engineRunID),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			driver.Value(int64(5000)),
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO simulation_runs`).
		WithArgs(driver.Value(runID)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectBegin()
	mock.ExpectPrepare(`INSERT INTO simulation_candidates`).
		ExpectExec().
		WithArgs(
			driver.Value(userID),
			sqlmock.AnyArg(),
			driver.Value(runID),
			driver.Value(runID), // candidate_id must equal parent run_id for ordinary fallback
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			driver.Value("scenario_yaml"),
			driver.Value(""),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	h := &Handler{
		engineClient: NewSimulationEngineClient(engineServer.URL),
		db:           db,
	}
	run := &domain.SimulationRun{
		RunID:           runID,
		EngineRunID:     engineRunID,
		UserID:          userID,
		ProjectPublicID: "proj-ordinary",
	}

	h.persistRunMetrics(context.Background(), run, "", nil)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPersistRunMetrics_BatchPersistsOptimizationReplayBundle(t *testing.T) {
	gin.SetMode(gin.TestMode)

	runID := "run-batch-1"
	engineRunID := "engine-batch-1"
	bestRunID := "opt-best"
	candidateRunID := "opt-cand-2"
	userID := "user-batch"

	engineServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+engineRunID:
			_, _ = w.Write([]byte(`{
  "run": {
    "id": "engine-batch-1",
    "status": "RUN_STATUS_COMPLETED",
    "best_run_id": "opt-best",
    "candidate_run_ids": ["opt-best", "opt-cand-2"],
    "optimization_replay": {
      "scenario_yaml_sha256": "sha-parent",
      "scenario_config_hash": "cfg-parent",
      "normalized_create_run_request": {"input": {"duration_ms": 2000}}
    }
  }
}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+engineRunID+"/export":
			_, _ = w.Write([]byte(`{
  "run": {"best_run_id": "opt-best"},
  "input": {"scenario_yaml": "hosts:\n  - id: parent\n    cores: 2\n", "duration_ms": 2000},
  "metrics": {"throughput_rps": 180}
}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+engineRunID+"/metrics/timeseries":
			_, _ = w.Write([]byte(`{"run_id":"engine-batch-1","points":[]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+engineRunID+"/metrics":
			_, _ = w.Write([]byte(`{"metrics":{"throughput_rps":180,"latency_p95_ms":20}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+bestRunID+"/export":
			_, _ = w.Write([]byte(`{
  "run": {"id": "opt-best"},
  "input": {"scenario_yaml": "hosts:\n  - id: best\n    cores: 4\n", "duration_ms": 2000, "seed": 11},
  "final_config": {"services": [{"service_id": "gateway-1", "replicas": 2}]},
  "metrics": {"throughput_rps": 180, "latency_p95_ms": 20}
}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+candidateRunID+"/export":
			_, _ = w.Write([]byte(`{
  "run": {"id": "opt-cand-2"},
  "input": {"scenario_yaml": "hosts:\n  - id: cand\n    cores: 3\n", "duration_ms": 2000, "seed": 12},
  "metrics": {"throughput_rps": 175, "latency_p95_ms": 25}
}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
		}
	}))
	defer engineServer.Close()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`INSERT INTO simulation_runs`).
		WithArgs(driver.Value(runID)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO simulation_summaries`).
		WithArgs(
			driver.Value(runID),
			driver.Value(engineRunID),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			driver.Value(int64(2000)),
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO simulation_runs`).
		WithArgs(driver.Value(runID)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectBegin()
	prep := mock.ExpectPrepare(`INSERT INTO simulation_candidates`)
	for _, tc := range []struct {
		id     string
		s3Path string
	}{
		{id: bestRunID, s3Path: "simulation/" + runID + "/best_scenario.yaml"},
		{id: candidateRunID, s3Path: ""},
	} {
		prep.ExpectExec().
			WithArgs(
				driver.Value(userID),
				sqlmock.AnyArg(),
				driver.Value(runID),
				driver.Value(tc.id),
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
				driver.Value("export"),
				driver.Value(tc.s3Path),
			).
			WillReturnResult(sqlmock.NewResult(1, 1))
	}
	mock.ExpectCommit()

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	runRepo := simrepo.NewRunRepository(rdb)
	svc := service.NewSimulationService(runRepo)
	run := &domain.SimulationRun{
		RunID:           runID,
		EngineRunID:     engineRunID,
		UserID:          userID,
		ProjectPublicID: "proj-batch",
		Status:          domain.StatusCompleted,
	}
	require.NoError(t, runRepo.Create(run))

	h := &Handler{
		simService:   svc,
		engineClient: NewSimulationEngineClient(engineServer.URL),
		db:           db,
	}

	h.persistRunMetrics(context.Background(), run, bestRunID, []string{bestRunID, candidateRunID})
	require.NoError(t, mock.ExpectationsWereMet())

	updated, err := svc.GetRun(runID)
	require.NoError(t, err)
	rawBundle, ok := updated.Metadata["optimization_replay_bundle"].(map[string]any)
	require.True(t, ok, "optimization_replay_bundle should be persisted in run metadata: %#v", updated.Metadata)
	assert.Equal(t, bestRunID, rawBundle["best_run_id"])
	assert.Equal(t, "cfg-parent", rawBundle["optimization_replay"].(map[string]any)["scenario_config_hash"])
	candidates, ok := rawBundle["candidates"].([]any)
	require.True(t, ok)
	require.Len(t, candidates, 2)
	first := candidates[0].(map[string]any)
	assert.Equal(t, bestRunID, first["candidate_id"])
	assert.Equal(t, true, first["is_best"])
	input := first["input"].(map[string]any)
	assert.Contains(t, input["scenario_yaml"], "best")
}
