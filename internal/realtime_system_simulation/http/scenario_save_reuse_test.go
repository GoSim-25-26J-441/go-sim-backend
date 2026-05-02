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
	"github.com/stretchr/testify/require"

	simrepo "github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/repository"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/service"
)

func TestCreateRunForProject_InlineScenarioWithoutSaveDoesNotTouchCache(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var validateCalls, runCalls int32
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/scenarios:validate":
			atomic.AddInt32(&validateCalls, 1)
			b, _ := io.ReadAll(r.Body)
			var payload map[string]any
			require.NoError(t, json.Unmarshal(b, &payload))
			require.Equal(t, "preflight", payload["mode"])
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
	h := &Handler{
		simService:        service.NewSimulationService(simrepo.NewRunRepository(rdb)),
		engineClient:      NewSimulationEngineClient(engine.URL),
		callbackURL:       "http://localhost/callback",
		db:                db,
		scenarioCacheRepo: simrepo.NewScenarioCacheRepository(db),
	}
	router := gin.New()
	router.POST("/projects/:project_id/runs", h.CreateRunForProject)

	yaml := minimalValidCoreScenarioYAML("svc-no-save")
	mock.ExpectQuery(`SELECT id FROM diagram_versions`).
		WithArgs("dv-1", "project-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("dv-1"))
	mock.ExpectExec(`INSERT INTO simulation_runs`).WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO simulation_summaries`).WithArgs(sqlmock.AnyArg(), "engine-1", sqlmock.AnyArg(), sqlmock.AnyArg(), yaml, nil, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))

	body := fmt.Sprintf(`{"diagram_version_id":"dv-1","scenario_yaml":%q,"save_scenario":false,"duration_ms":1000}`, yaml)
	req := httptest.NewRequest(http.MethodPost, "/projects/project-1/runs", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
	require.Equal(t, int32(1), atomic.LoadInt32(&validateCalls))
	require.Equal(t, int32(1), atomic.LoadInt32(&runCalls))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateRunForProject_SaveScenarioTruePersistsEdited(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var validateModes []string
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/scenarios:validate":
			b, _ := io.ReadAll(r.Body)
			var payload map[string]any
			require.NoError(t, json.Unmarshal(b, &payload))
			validateModes = append(validateModes, payload["mode"].(string))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"valid":true,"errors":[],"warnings":[],"summary":{"hosts":1,"services":1,"workloads":1}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
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
	yaml := minimalValidCoreScenarioYAML("svc-save-inline")
	hashBytes := sha256.Sum256([]byte(yaml))
	hashHex := hex.EncodeToString(hashBytes[:])

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	h := &Handler{
		simService:        service.NewSimulationService(simrepo.NewRunRepository(rdb)),
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
		WillReturnRows(sqlmock.NewRows([]string{"yaml_content"}).AddRow("services:\n  - id: g\n    type: api_gateway\n"))
	mock.ExpectQuery(`SELECT diagram_version_id, scenario_yaml, scenario_hash`).
		WithArgs("dv-1").
		WillReturnRows(sqlmock.NewRows([]string{"diagram_version_id", "scenario_yaml", "scenario_hash", "s3_path", "source", "source_hash", "created_at", "updated_at"}))
	mock.ExpectQuery(`INSERT INTO simulation_scenario_cache`).
		WithArgs("dv-1", yaml, sqlmock.AnyArg(), "", "edited", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"diagram_version_id", "scenario_yaml", "scenario_hash", "s3_path", "source", "source_hash", "created_at", "updated_at"}).
			AddRow("dv-1", yaml, hashHex, nil, "edited", nil, now, now))
	mock.ExpectExec(`INSERT INTO simulation_runs`).WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO simulation_summaries`).WithArgs(sqlmock.AnyArg(), "engine-1", sqlmock.AnyArg(), sqlmock.AnyArg(), yaml, nil, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))

	body := fmt.Sprintf(`{"diagram_version_id":"dv-1","scenario_yaml":%q,"save_scenario":true,"duration_ms":1000}`, yaml)
	req := httptest.NewRequest(http.MethodPost, "/projects/project-1/runs", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
	require.Equal(t, []string{"draft", "preflight"}, validateModes)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateRunForProject_InvalidScenarioYAMLRejectedByEngineBeforeRun(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var validateCalls, runCalls int32
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/scenarios:validate":
			atomic.AddInt32(&validateCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"valid":false,"errors":[{"code":"PARSE","message":"invalid scenario"}],"warnings":[],"summary":{}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			atomic.AddInt32(&runCalls, 1)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"run":{"id":"engine-1"}}`))
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
	h := &Handler{
		simService:        service.NewSimulationService(simrepo.NewRunRepository(rdb)),
		engineClient:      NewSimulationEngineClient(engine.URL),
		scenarioCacheRepo: simrepo.NewScenarioCacheRepository(db),
	}
	router := gin.New()
	router.POST("/projects/:project_id/runs", h.CreateRunForProject)

	mock.ExpectQuery(`SELECT id FROM diagram_versions`).
		WithArgs("dv-1", "project-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("dv-1"))

	req := httptest.NewRequest(http.MethodPost, "/projects/project-1/runs", bytes.NewBufferString(`{"diagram_version_id":"dv-1","scenario_yaml":"hosts: []","duration_ms":1000}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	require.GreaterOrEqual(t, atomic.LoadInt32(&validateCalls), int32(1))
	require.Equal(t, int32(0), atomic.LoadInt32(&runCalls))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateRunForProject_SaveScenarioRequiresScenarioYAML(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	h := &Handler{
		simService:        service.NewSimulationService(simrepo.NewRunRepository(rdb)),
		scenarioCacheRepo: simrepo.NewScenarioCacheRepository(db),
	}
	router := gin.New()
	router.POST("/projects/:project_id/runs", h.CreateRunForProject)

	mock.ExpectQuery(`SELECT id FROM diagram_versions`).
		WithArgs("dv-1", "project-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("dv-1"))

	req := httptest.NewRequest(http.MethodPost, "/projects/project-1/runs", bytes.NewBufferString(`{"diagram_version_id":"dv-1","save_scenario":true,"duration_ms":1000}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Contains(t, resp["error"], "save_scenario requires scenario_yaml")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPutDiagramVersionScenario_ThenGetReturnsSavedYAML(t *testing.T) {
	gin.SetMode(gin.TestMode)
	validYAML := minimalValidCoreScenarioYAML("svc-put-get")
	now := time.Now()
	hashBytes := sha256.Sum256([]byte(validYAML))
	hashHex := hex.EncodeToString(hashBytes[:])
	diagramYAML := "services:\n  - id: g\n    type: api_gateway\n"
	srcHash := simrepo.HashAMGAPDSource(diagramYAML)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/scenarios:validate" {
			b, _ := io.ReadAll(r.Body)
			var payload map[string]any
			require.NoError(t, json.Unmarshal(b, &payload))
			require.Equal(t, "draft", payload["mode"])
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"valid":true,"errors":[],"warnings":[],"summary":{"hosts":1,"services":1,"workloads":1}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer engine.Close()
	h := &Handler{
		simService:        service.NewSimulationService(simrepo.NewRunRepository(rdb)),
		engineClient:      NewSimulationEngineClient(engine.URL),
		scenarioCacheRepo: simrepo.NewScenarioCacheRepository(db),
	}
	router := gin.New()
	router.PUT("/projects/:project_id/diagram-versions/:diagram_version_id/scenario", h.PutDiagramVersionScenario)
	router.GET("/projects/:project_id/diagram-versions/:diagram_version_id/scenario", h.GetDiagramVersionScenario)

	mock.ExpectQuery(`SELECT id FROM diagram_versions`).
		WithArgs("dv-1", "p-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("dv-1"))
	mock.ExpectQuery(`SELECT yaml_content FROM diagram_versions`).
		WithArgs("dv-1", "p-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"yaml_content"}).AddRow(diagramYAML))
	mock.ExpectQuery(`SELECT diagram_version_id, scenario_yaml, scenario_hash`).
		WithArgs("dv-1").
		WillReturnRows(sqlmock.NewRows([]string{"diagram_version_id", "scenario_yaml", "scenario_hash", "s3_path", "source", "source_hash", "created_at", "updated_at"}))
	mock.ExpectQuery(`INSERT INTO simulation_scenario_cache`).
		WithArgs("dv-1", validYAML, sqlmock.AnyArg(), "", "edited", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"diagram_version_id", "scenario_yaml", "scenario_hash", "s3_path", "source", "source_hash", "created_at", "updated_at"}).
			AddRow("dv-1", validYAML, hashHex, nil, "edited", srcHash, now, now))

	putBody := fmt.Sprintf(`{"scenario_yaml":%q,"overwrite":false}`, validYAML)
	reqPut := httptest.NewRequest(http.MethodPut, "/projects/p-1/diagram-versions/dv-1/scenario", bytes.NewBufferString(putBody))
	reqPut.Header.Set("Content-Type", "application/json")
	reqPut.Header.Set("X-User-Id", "user-1")
	wPut := httptest.NewRecorder()
	router.ServeHTTP(wPut, reqPut)
	require.Equal(t, http.StatusOK, wPut.Code)

	mock.ExpectQuery(`SELECT id FROM diagram_versions`).
		WithArgs("dv-1", "p-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("dv-1"))
	mock.ExpectQuery(`SELECT yaml_content FROM diagram_versions`).
		WithArgs("dv-1", "p-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"yaml_content"}).AddRow(diagramYAML))
	mock.ExpectQuery(`SELECT c.diagram_version_id, c.scenario_yaml, c.scenario_hash`).
		WithArgs("dv-1", "p-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"diagram_version_id", "scenario_yaml", "scenario_hash", "s3_path", "source", "source_hash", "created_at", "updated_at"}).
			AddRow("dv-1", validYAML, hashHex, nil, "edited", srcHash, now, now))

	reqGet := httptest.NewRequest(http.MethodGet, "/projects/p-1/diagram-versions/dv-1/scenario", nil)
	reqGet.Header.Set("X-User-Id", "user-1")
	wGet := httptest.NewRecorder()
	router.ServeHTTP(wGet, reqGet)
	require.Equal(t, http.StatusOK, wGet.Code)
	var got map[string]any
	require.NoError(t, json.Unmarshal(wGet.Body.Bytes(), &got))
	require.Equal(t, validYAML, got["scenario_yaml"])
	require.Equal(t, "edited", got["source"])
	require.NoError(t, mock.ExpectationsWereMet())
}
