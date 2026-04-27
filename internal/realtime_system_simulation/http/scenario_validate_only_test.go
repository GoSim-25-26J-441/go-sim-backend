package http

import (
	"bytes"
	"context"
	"encoding/json"
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

type countingStorage struct {
	putCalls int32
}

func (s *countingStorage) PutObject(_ context.Context, _ string, _ []byte) error {
	atomic.AddInt32(&s.putCalls, 1)
	return nil
}

func (s *countingStorage) GetObject(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}

func TestRegister_ExposesScenarioValidateRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	h := &Handler{}
	h.Register(router.Group("/api/v1/simulation"))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/simulation/projects/p-1/diagram-versions/dv-1/scenario/validate", bytes.NewBufferString(`{"scenario_yaml":"hosts: []"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.NotEqual(t, http.StatusNotFound, w.Code)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestPostValidateDiagramVersionScenario_Valid_ReturnsRawValidation_NoPersistence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var validateCalls int32
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/scenarios:validate" {
			atomic.AddInt32(&validateCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"valid":true,"errors":[],"warnings":[],"summary":{"hosts":1,"services":2,"workloads":1}}`))
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
	st := &countingStorage{}

	h := &Handler{
		simService:        service.NewSimulationService(simrepo.NewRunRepository(rdb)),
		engineClient:      NewSimulationEngineClient(engine.URL),
		scenarioCacheRepo: simrepo.NewScenarioCacheRepository(db),
		s3Client:          st,
	}
	router := gin.New()
	router.POST("/projects/:project_id/diagram-versions/:diagram_version_id/scenario/validate", h.PostValidateDiagramVersionScenario)

	mock.ExpectQuery(`SELECT id FROM diagram_versions`).
		WithArgs("dv-1", "p-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("dv-1"))

	yaml := minimalValidCoreScenarioYAML("svc-validate")
	req := httptest.NewRequest(http.MethodPost, "/projects/p-1/diagram-versions/dv-1/scenario/validate", bytes.NewBufferString(`{"scenario_yaml":`+jsonEscape(yaml)+`}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, true, resp["valid"])
	require.Equal(t, int32(1), atomic.LoadInt32(&validateCalls))
	require.Equal(t, int32(0), atomic.LoadInt32(&st.putCalls))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPostValidateDiagramVersionScenario_Invalid_ReturnsStructuredValidation422(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/scenarios:validate" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"valid":false,"errors":[{"code":"TARGET_NOT_FOUND","message":"unknown target","path":"workload[0].to"}],"warnings":[],"summary":{"hosts":1}}`))
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
	router.POST("/projects/:project_id/diagram-versions/:diagram_version_id/scenario/validate", h.PostValidateDiagramVersionScenario)

	mock.ExpectQuery(`SELECT id FROM diagram_versions`).
		WithArgs("dv-1", "p-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("dv-1"))

	req := httptest.NewRequest(http.MethodPost, "/projects/p-1/diagram-versions/dv-1/scenario/validate", bytes.NewBufferString(`{"scenario_yaml":"hosts: []"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "invalid scenario_yaml", resp["error"])
	require.NotNil(t, resp["validation"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPostValidateDiagramVersionScenario_EmptyScenarioYAML_Returns400(t *testing.T) {
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
	router.POST("/projects/:project_id/diagram-versions/:diagram_version_id/scenario/validate", h.PostValidateDiagramVersionScenario)

	req := httptest.NewRequest(http.MethodPost, "/projects/p-1/diagram-versions/dv-1/scenario/validate", bytes.NewBufferString(`{"scenario_yaml":"   "}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPostValidateDiagramVersionScenario_InvalidOwnership_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/scenarios:validate" {
			t.Fatalf("engine validate should not be called for invalid ownership")
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
	router.POST("/projects/:project_id/diagram-versions/:diagram_version_id/scenario/validate", h.PostValidateDiagramVersionScenario)

	mock.ExpectQuery(`SELECT id FROM diagram_versions`).
		WithArgs("dv-1", "p-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	req := httptest.NewRequest(http.MethodPost, "/projects/p-1/diagram-versions/dv-1/scenario/validate", bytes.NewBufferString(`{"scenario_yaml":"hosts: []"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}
