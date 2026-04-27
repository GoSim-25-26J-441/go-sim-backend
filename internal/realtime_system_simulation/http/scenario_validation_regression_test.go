package http

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	simrepo "github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/repository"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/service"
)

// Regression: invalid semantic scenario from engine must not hit cache INSERT and must surface structured validation (incl. path).
func TestGetDiagramVersionScenario_InvalidWorkloadEndpoint_NoCacheInsert_StructuredErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var validateCalls int32
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/scenarios:validate" {
			atomic.AddInt32(&validateCalls, 1)
			b, _ := io.ReadAll(r.Body)
			var req map[string]any
			require.NoError(t, json.Unmarshal(b, &req))
			require.Equal(t, "preflight", req["mode"])
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{
  "valid": false,
  "errors": [
    {
      "code": "UNKNOWN_WORKLOAD_ENDPOINT",
      "message": "workload target checkout:/write references missing endpoint /write on service checkout",
      "path": "workload[0].to"
    }
  ],
  "warnings": []
}`))
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
	h := &Handler{
		simService:        simSvc,
		engineClient:      NewSimulationEngineClient(engine.URL),
		scenarioCacheRepo: simrepo.NewScenarioCacheRepository(db),
	}
	router := gin.New()
	router.GET("/projects/:project_id/diagram-versions/:diagram_version_id/scenario", h.GetDiagramVersionScenario)

	mock.ExpectQuery(`SELECT id FROM diagram_versions`).
		WithArgs("dv-1", "p-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("dv-1"))
	mock.ExpectQuery(`SELECT yaml_content FROM diagram_versions`).
		WithArgs("dv-1", "p-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"yaml_content"}).AddRow(minimalAMGForScenarioGET))
	mock.ExpectQuery(`SELECT c.diagram_version_id, c.scenario_yaml, c.scenario_hash`).
		WithArgs("dv-1", "p-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"diagram_version_id", "scenario_yaml", "scenario_hash", "s3_path", "source", "source_hash", "created_at", "updated_at"}))

	req := httptest.NewRequest(http.MethodGet, "/projects/p-1/diagram-versions/dv-1/scenario", nil)
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	require.GreaterOrEqual(t, atomic.LoadInt32(&validateCalls), int32(1))
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	val, ok := resp["validation"].(map[string]any)
	require.True(t, ok, "expected validation object")
	errs, ok := val["errors"].([]any)
	require.True(t, ok)
	require.GreaterOrEqual(t, len(errs), 1)
	e0 := errs[0].(map[string]any)
	require.Equal(t, "UNKNOWN_WORKLOAD_ENDPOINT", e0["code"])
	require.Equal(t, "workload[0].to", e0["path"])
	require.Contains(t, resp, "draft_scenario_yaml")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateRunForProject_SemanticInvalid_NoBackendRun_NoEngineRuns(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var validateCalls, engineRunCalls int32
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/scenarios:validate":
			atomic.AddInt32(&validateCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
  "valid": false,
  "errors": [{"code":"UNKNOWN_WORKLOAD_ENDPOINT","message":"missing endpoint","path":"workload[0].to"}],
  "warnings": []
}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			atomic.AddInt32(&engineRunCalls, 1)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"run":{"id":"should-not-be-called"}}`))
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

	badYAML := `services:
  - id: checkout
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
    to: checkout:/write
    arrival:
      type: poisson
      rate_rps: 10
`
	body := `{"diagram_version_id":"dv-1","scenario_yaml":` + jsonEscape(badYAML) + `,"duration_ms":1000}`
	req := httptest.NewRequest(http.MethodPost, "/projects/project-1/runs", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	require.GreaterOrEqual(t, atomic.LoadInt32(&validateCalls), int32(1))
	require.Equal(t, int32(0), atomic.LoadInt32(&engineRunCalls))
	runIDs, listErr := simSvc.ListRunsByUser("user-1")
	require.NoError(t, listErr)
	require.Empty(t, runIDs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func jsonEscape(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}
