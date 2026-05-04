package http

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	simrepo "github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/repository"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/service"
)

// minimalValidCoreScenarioYAML returns a minimal simulation-core-valid scenario for tests.
func minimalValidCoreScenarioYAML(serviceID string) string {
	return fmt.Sprintf(`hosts:
  - id: host-1
    cores: 4
services:
  - id: %s
    replicas: 1
    model: cpu
    endpoints:
      - path: /read
        mean_cpu_ms: 1
        cpu_sigma_ms: 0
        net_latency_ms: {mean: 1, sigma: 0.1}
        downstream: []
workload:
  - from: client
    to: %s:/read
    arrival:
      type: poisson
      rate_rps: 10
`, serviceID, serviceID)
}

func TestCreateRunForProject_CachesAndReusesScenario(t *testing.T) {
	gin.SetMode(gin.TestMode)
	validYAML := minimalValidCoreScenarioYAML("svc-first")
	var capturedFirst, capturedSecond []byte
	runCall := 0
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/scenarios:validate":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"valid":true,"errors":[],"warnings":[],"summary":{"hosts":1,"services":1,"workloads":1}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			body, _ := io.ReadAll(r.Body)
			if runCall == 0 {
				capturedFirst = body
			} else {
				capturedSecond = body
			}
			runCall++
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"run":{"id":"engine-1","status":"RUN_STATUS_PENDING","created_at_unix_ms":0}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer engine.Close()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	now := time.Now()
	hashFirstBytes := sha256.Sum256([]byte(validYAML))
	hashFirst := hex.EncodeToString(hashFirstBytes[:])
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	runRepo := simrepo.NewRunRepository(rdb)
	simSvc := service.NewSimulationService(runRepo)

	h := &Handler{
		simService:        simSvc,
		engineClient:      NewSimulationEngineClient(engine.URL),
		callbackURL:       "http://localhost/callback",
		db:                db,
		scenarioCacheRepo: simrepo.NewScenarioCacheRepository(db),
	}
	router := gin.New()
	router.POST("/projects/:project_id/runs", h.CreateRunForProject)

	mock.ExpectQuery(`SELECT id FROM diagram_versions`).
		WithArgs("dv-1", "project-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("dv-1"))
	mock.ExpectQuery(`SELECT yaml_content FROM diagram_versions`).
		WithArgs("dv-1", "project-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"yaml_content"}).AddRow("services:\n  - id: x\n    type: api_gateway\n"))
	mock.ExpectQuery(`SELECT diagram_version_id, scenario_yaml, scenario_hash`).
		WithArgs("dv-1").
		WillReturnRows(sqlmock.NewRows([]string{"diagram_version_id", "scenario_yaml", "scenario_hash", "s3_path", "source", "source_hash", "created_at", "updated_at"}))
	mock.ExpectQuery(`INSERT INTO simulation_scenario_cache`).
		WithArgs("dv-1", validYAML, sqlmock.AnyArg(), "", "edited", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"diagram_version_id", "scenario_yaml", "scenario_hash", "s3_path", "source", "source_hash", "created_at", "updated_at"}).
			AddRow("dv-1", validYAML, hashFirst, nil, "edited", nil, now, now))
	mock.ExpectExec(`INSERT INTO simulation_runs`).WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO simulation_summaries`).WithArgs(sqlmock.AnyArg(), "engine-1", sqlmock.AnyArg(), sqlmock.AnyArg(), validYAML, nil, true, "{}").WillReturnResult(sqlmock.NewResult(1, 1))

	firstBody := fmt.Sprintf(`{"diagram_version_id":"dv-1","scenario_yaml":%q,"save_scenario":true,"duration_ms":1000}`, validYAML)
	req := httptest.NewRequest(http.MethodPost, "/projects/project-1/runs", bytes.NewBufferString(firstBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	mock.ExpectQuery(`SELECT id FROM diagram_versions`).
		WithArgs("dv-1", "project-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("dv-1"))
	mock.ExpectQuery(`SELECT yaml_content FROM diagram_versions`).
		WithArgs("dv-1", "project-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"yaml_content"}).AddRow("services:\n  - id: x\n    type: api_gateway\n"))
	mock.ExpectQuery(`SELECT c.diagram_version_id, c.scenario_yaml, c.scenario_hash`).
		WithArgs("dv-1", "project-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"diagram_version_id", "scenario_yaml", "scenario_hash", "s3_path", "source", "source_hash", "created_at", "updated_at"}).
			AddRow("dv-1", validYAML, hashFirst, nil, "edited", nil, now, now))
	mock.ExpectExec(`INSERT INTO simulation_runs`).WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO simulation_summaries`).WithArgs(sqlmock.AnyArg(), "engine-1", sqlmock.AnyArg(), sqlmock.AnyArg(), validYAML, nil, true, "{}").WillReturnResult(sqlmock.NewResult(1, 1))

	secondBody := `{"diagram_version_id":"dv-1","duration_ms":1000}`
	req2 := httptest.NewRequest(http.MethodPost, "/projects/project-1/runs", bytes.NewBufferString(secondBody))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-User-Id", "user-1")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusCreated, w2.Code)

	var firstPayload, secondPayload map[string]any
	require.NoError(t, json.Unmarshal(capturedFirst, &firstPayload))
	require.NoError(t, json.Unmarshal(capturedSecond, &secondPayload))
	assert.Equal(t, validYAML, firstPayload["input"].(map[string]any)["scenario_yaml"])
	assert.Equal(t, validYAML, secondPayload["input"].(map[string]any)["scenario_yaml"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateRunForProject_MissingScenarioAndCacheReturns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	runRepo := simrepo.NewRunRepository(rdb)
	simSvc := service.NewSimulationService(runRepo)
	h := &Handler{simService: simSvc, scenarioCacheRepo: simrepo.NewScenarioCacheRepository(db), db: db}
	router := gin.New()
	router.POST("/projects/:project_id/runs", h.CreateRunForProject)

	mock.ExpectQuery(`SELECT id FROM diagram_versions`).
		WithArgs("dv-1", "project-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("dv-1"))
	mock.ExpectQuery(`SELECT yaml_content FROM diagram_versions`).
		WithArgs("dv-1", "project-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"yaml_content"}).AddRow(nil))

	req := httptest.NewRequest(http.MethodPost, "/projects/project-1/runs", bytes.NewBufferString(`{"diagram_version_id":"dv-1"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	runIDs, listErr := runRepo.ListByProjectID("project-1")
	require.NoError(t, listErr)
	require.Empty(t, runIDs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateRunForProject_ScenarioConflictReturns409(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var validateCalls, runCalls int32
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/scenarios:validate":
			atomic.AddInt32(&validateCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"valid":true,"errors":[],"warnings":[],"summary":{"hosts":1,"services":1,"workloads":1}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			atomic.AddInt32(&runCalls, 1)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"run":{"id":"engine-1","status":"RUN_STATUS_PENDING","created_at_unix_ms":0}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer engine.Close()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	now := time.Now()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	runRepo := simrepo.NewRunRepository(rdb)
	simSvc := service.NewSimulationService(runRepo)
	h := &Handler{
		simService:        simSvc,
		engineClient:      NewSimulationEngineClient(engine.URL),
		callbackURL:       "http://localhost/callback",
		db:                db,
		scenarioCacheRepo: simrepo.NewScenarioCacheRepository(db),
	}
	router := gin.New()
	router.POST("/projects/:project_id/runs", h.CreateRunForProject)

	changedYAML := minimalValidCoreScenarioYAML("svc-other")
	mock.ExpectQuery(`SELECT id FROM diagram_versions`).WithArgs("dv-1", "project-1", "user-1").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("dv-1"))
	mock.ExpectQuery(`SELECT yaml_content FROM diagram_versions`).
		WithArgs("dv-1", "project-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"yaml_content"}).AddRow("services:\n  - id: z\n    type: api_gateway\n"))
	mock.ExpectQuery(`SELECT diagram_version_id, scenario_yaml, scenario_hash`).
		WithArgs("dv-1").
		WillReturnRows(sqlmock.NewRows([]string{"diagram_version_id", "scenario_yaml", "scenario_hash", "s3_path", "source", "source_hash", "created_at", "updated_at"}).
			AddRow("dv-1", "old", "old-hash", nil, "edited", nil, now, now))

	req := httptest.NewRequest(http.MethodPost, "/projects/project-1/runs", bytes.NewBufferString(fmt.Sprintf(`{"diagram_version_id":"dv-1","scenario_yaml":%q,"save_scenario":true,"duration_ms":1000}`, changedYAML)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusConflict, w.Code)
	require.GreaterOrEqual(t, atomic.LoadInt32(&validateCalls), int32(1))
	require.Equal(t, int32(0), atomic.LoadInt32(&runCalls))
	runIDs, listErr := runRepo.ListByProjectID("project-1")
	require.NoError(t, listErr)
	require.Empty(t, runIDs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateRunForProject_InvalidDiagramVersionReturns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var validateCalls, runCalls int32
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/scenarios:validate":
			atomic.AddInt32(&validateCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"valid":true,"errors":[],"warnings":[],"summary":{"hosts":1,"services":1,"workloads":1}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			atomic.AddInt32(&runCalls, 1)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"run":{"id":"engine-1","status":"RUN_STATUS_PENDING","created_at_unix_ms":0}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer engine.Close()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	runRepo := simrepo.NewRunRepository(rdb)
	simSvc := service.NewSimulationService(runRepo)
	h := &Handler{
		simService:        simSvc,
		engineClient:      NewSimulationEngineClient(engine.URL),
		callbackURL:       "http://localhost/callback",
		db:                db,
		scenarioCacheRepo: simrepo.NewScenarioCacheRepository(db),
	}
	router := gin.New()
	router.POST("/projects/:project_id/runs", h.CreateRunForProject)

	mock.ExpectQuery(`SELECT id FROM diagram_versions`).
		WithArgs("dv-missing", "project-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	yaml := minimalValidCoreScenarioYAML("svc-x")
	req := httptest.NewRequest(http.MethodPost, "/projects/project-1/runs", bytes.NewBufferString(fmt.Sprintf(`{"diagram_version_id":"dv-missing","scenario_yaml":%q,"duration_ms":1000}`, yaml)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Equal(t, int32(0), atomic.LoadInt32(&validateCalls))
	require.Equal(t, int32(0), atomic.LoadInt32(&runCalls))
	runIDs, listErr := runRepo.ListByProjectID("project-1")
	require.NoError(t, listErr)
	require.Empty(t, runIDs)
	require.NoError(t, mock.ExpectationsWereMet())
}
