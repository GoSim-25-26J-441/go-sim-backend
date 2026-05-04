package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	simrepo "github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/repository"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/service"
)

// placementStressYAML parses cleanly but is typically rejected by resource preflight (2 replicas, 1 core host).
func placementStressYAML() string {
	return `hosts:
  - id: host-1
    cores: 1
services:
  - id: api
    replicas: 2
    model: cpu
    endpoints:
      - path: /read
        mean_cpu_ms: 1
        cpu_sigma_ms: 0
        net_latency_ms: {mean: 1, sigma: 0.1}
        downstream: []
workload:
  - from: client
    to: api:/read
    arrival:
      type: poisson
      rate_rps: 1
`
}

func TestPreflightValidation_CreateRunForProject_ValidProceeds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var validateCalls, runCalls int32
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/scenarios:validate":
			atomic.AddInt32(&validateCalls, 1)
			b, _ := io.ReadAll(r.Body)
			var payload map[string]any
			require.NoError(t, json.Unmarshal(b, &payload))
			assert.Contains(t, payload, "scenario_yaml")
			require.Equal(t, "preflight", payload["mode"])
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"valid":true,"errors":[],"warnings":[],"summary":{"hosts":1,"services":1,"workloads":1}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			atomic.AddInt32(&runCalls, 1)
			w.Header().Set("Content-Type", "application/json")
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
	simSvc := service.NewSimulationService(simrepo.NewRunRepository(rdb))
	yaml := minimalValidCoreScenarioYAML("svc-preflight-ok")
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
	mock.ExpectExec(`INSERT INTO simulation_runs`).WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO simulation_summaries`).WithArgs(sqlmock.AnyArg(), "engine-1", sqlmock.AnyArg(), sqlmock.AnyArg(), yaml, nil, true, "{}").WillReturnResult(sqlmock.NewResult(1, 1))

	body := fmt.Sprintf(`{"diagram_version_id":"dv-1","scenario_yaml":%q,"duration_ms":1000}`, yaml)
	req := httptest.NewRequest(http.MethodPost, "/projects/project-1/runs", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
	require.GreaterOrEqual(t, atomic.LoadInt32(&validateCalls), int32(1))
	require.Equal(t, int32(1), atomic.LoadInt32(&runCalls))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPreflightValidation_CreateRunForProject_InvalidPreflightNoRunNoCache(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var validateCalls, runCalls int32
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/scenarios:validate":
			atomic.AddInt32(&validateCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{
  "valid": false,
  "errors": [
    {
      "code": "PLACEMENT_INFEASIBLE",
      "message": "cannot place service api: insufficient host capacity",
      "service_id": "api"
    }
  ],
  "warnings": [],
  "summary": {"hosts": 1, "services": 1, "workloads": 1}
}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			atomic.AddInt32(&runCalls, 1)
			w.WriteHeader(http.StatusCreated)
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
	simSvc := service.NewSimulationService(simrepo.NewRunRepository(rdb))
	yaml := placementStressYAML()
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

	body := fmt.Sprintf(`{"diagram_version_id":"dv-1","scenario_yaml":%q,"duration_ms":1000}`, yaml)
	req := httptest.NewRequest(http.MethodPost, "/projects/project-1/runs", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "invalid scenario_yaml", resp["error"])
	require.NotNil(t, resp["validation"])
	require.GreaterOrEqual(t, atomic.LoadInt32(&validateCalls), int32(1))
	require.Equal(t, int32(0), atomic.LoadInt32(&runCalls))
	runIDs, listErr := simSvc.ListRunsByUser("user-1")
	require.NoError(t, listErr)
	require.Empty(t, runIDs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPreflightValidation_PutScenarioFails_NoCacheUpsert(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/scenarios:validate" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"valid":false,"errors":[{"code":"X","message":"bad"}],"warnings":[],"summary":{}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer engine.Close()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	h := &Handler{
		simService:        service.NewSimulationService(simrepo.NewRunRepository(rdb)),
		engineClient:      NewSimulationEngineClient(engine.URL),
		scenarioCacheRepo: simrepo.NewScenarioCacheRepository(db),
	}
	router := gin.New()
	router.PUT("/projects/:project_id/diagram-versions/:diagram_version_id/scenario", h.PutDiagramVersionScenario)

	// Preflight fails before any DB access (validate runs before VerifyDiagramVersion).
	yaml := minimalValidCoreScenarioYAML("svc-put-fail")
	putBody := fmt.Sprintf(`{"scenario_yaml":%q,"overwrite":false}`, yaml)
	req := httptest.NewRequest(http.MethodPut, "/projects/p-1/diagram-versions/dv-1/scenario", bytes.NewBufferString(putBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPreflightValidation_SaveScenarioTrueFailsBeforeCacheUpsert(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/scenarios:validate" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"valid":false,"errors":[{"code":"X","message":"bad"}],"warnings":[],"summary":{}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer engine.Close()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	simSvc := service.NewSimulationService(simrepo.NewRunRepository(rdb))
	yaml := minimalValidCoreScenarioYAML("svc-save-fail")
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

	body := fmt.Sprintf(`{"diagram_version_id":"dv-1","scenario_yaml":%q,"save_scenario":true,"duration_ms":1000}`, yaml)
	req := httptest.NewRequest(http.MethodPost, "/projects/project-1/runs", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	runIDs, listErr := simSvc.ListRunsByUser("user-1")
	require.NoError(t, listErr)
	require.Empty(t, runIDs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPreflightValidation_EngineValidateUnavailable_Returns503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/scenarios:validate" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"internal"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer engine.Close()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	simSvc := service.NewSimulationService(simrepo.NewRunRepository(rdb))
	yaml := minimalValidCoreScenarioYAML("svc-unavail")
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

	body := fmt.Sprintf(`{"diagram_version_id":"dv-1","scenario_yaml":%q,"duration_ms":1000}`, yaml)
	req := httptest.NewRequest(http.MethodPost, "/projects/project-1/runs", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	runIDs, listErr := simSvc.ListRunsByUser("user-1")
	require.NoError(t, listErr)
	require.Empty(t, runIDs)
	require.NoError(t, mock.ExpectationsWereMet())
}
