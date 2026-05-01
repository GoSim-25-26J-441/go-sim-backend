package http

import (
	"encoding/json"
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

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/analysis_suggestions/hostconfig"
	simrepo "github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/repository"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/service"
)

// Verifies GET scenario uses stored analysis host sizing when request_responses has valid JSON.
func TestGetDiagramVersionScenario_usesStoredHostConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var validateCalls int32
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/scenarios:validate" {
			atomic.AddInt32(&validateCalls, 1)
			b, _ := io.ReadAll(r.Body)
			var payload map[string]any
			require.NoError(t, json.Unmarshal(b, &payload))
			require.Contains(t, payload, "scenario_yaml")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"valid":true,"errors":[],"warnings":[],"summary":{"hosts":3,"services":1,"workloads":1}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
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

	h := &Handler{
		simService:        service.NewSimulationService(simrepo.NewRunRepository(rdb)),
		engineClient:      NewSimulationEngineClient(engine.URL),
		scenarioCacheRepo: simrepo.NewScenarioCacheRepository(db),
		db:                db,
	}
	router := gin.New()
	router.GET("/projects/:project_id/diagram-versions/:diagram_version_id/scenario", h.GetDiagramVersionScenario)

	reqJSON := `{"design":{"preferred_vcpu":4,"preferred_memory_gb":16},"simulation":{"nodes":3}}`

	mock.ExpectQuery(`SELECT id FROM diagram_versions`).
		WithArgs("dv-1", "p-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("dv-1"))
	mock.ExpectQuery(`SELECT yaml_content FROM diagram_versions`).
		WithArgs("dv-1", "p-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"yaml_content"}).AddRow(minimalAMGForScenarioGET))
	mock.ExpectQuery(`SELECT c.diagram_version_id, c.scenario_yaml, c.scenario_hash`).
		WithArgs("dv-1", "p-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"diagram_version_id", "scenario_yaml", "scenario_hash", "s3_path", "source", "source_hash", "created_at", "updated_at"}))
	mock.ExpectQuery(`SELECT request FROM request_responses WHERE user_id = \$1 AND COALESCE\(project_id, ''\) = \$2 AND run_id IS NULL ORDER BY created_at DESC LIMIT 1`).
		WithArgs("user-1", "p-1").
		WillReturnRows(sqlmock.NewRows([]string{"request"}).AddRow(reqJSON))
	mock.ExpectQuery(`SELECT diagram_version_id, scenario_yaml, scenario_hash`).
		WithArgs("dv-1").
		WillReturnRows(sqlmock.NewRows([]string{"diagram_version_id", "scenario_yaml", "scenario_hash", "s3_path", "source", "source_hash", "created_at", "updated_at"}))
	mock.ExpectQuery(`INSERT INTO simulation_scenario_cache`).
		WithArgs("dv-1", sqlmock.AnyArg(), sqlmock.AnyArg(), "", "generated", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"diagram_version_id", "scenario_yaml", "scenario_hash", "s3_path", "source", "source_hash", "created_at", "updated_at"}).
			AddRow("dv-1", "yaml", "h", nil, "generated", "sh", now, now))

	req := httptest.NewRequest(http.MethodGet, "/projects/p-1/diagram-versions/dv-1/scenario", nil)
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.GreaterOrEqual(t, atomic.LoadInt32(&validateCalls), int32(1))
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	yamlStr, _ := resp["scenario_yaml"].(string)
	require.Contains(t, yamlStr, "cores: 4")
	require.Contains(t, yamlStr, "memory_gb: 16")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetDiagramVersionScenario_differentHostConfigInvalidatesGeneratedCache(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/scenarios:validate" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"valid":true}`))
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
		db:                db,
	}
	router := gin.New()
	router.GET("/projects/:project_id/diagram-versions/:diagram_version_id/scenario", h.GetDiagramVersionScenario)

	newCfg := `{"design":{"preferred_vcpu":4,"preferred_memory_gb":16},"simulation":{"nodes":3}}`

	mock.ExpectQuery(`SELECT id FROM diagram_versions`).
		WithArgs("dv-1", "p-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("dv-1"))
	mock.ExpectQuery(`SELECT yaml_content FROM diagram_versions`).
		WithArgs("dv-1", "p-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"yaml_content"}).AddRow(minimalAMGForScenarioGET))

	oldCanon := hostconfig.CanonicalJSON(hostconfig.ScenarioHostConfig{Nodes: 3, Cores: 8, MemoryGB: 32})
	oldGenHash := simrepo.HashScenarioGenerationSource(minimalAMGForScenarioGET, oldCanon)
	require.NotEqual(t, simrepo.HashAMGAPDSource(minimalAMGForScenarioGET), oldGenHash)

	now := time.Now()
	mock.ExpectQuery(`SELECT c.diagram_version_id, c.scenario_yaml, c.scenario_hash`).
		WithArgs("dv-1", "p-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"diagram_version_id", "scenario_yaml", "scenario_hash", "s3_path", "source", "source_hash", "created_at", "updated_at"}).
			AddRow("dv-1", "cached-yaml", "h", nil, "generated", oldGenHash, now, now))

	mock.ExpectQuery(`SELECT request FROM request_responses WHERE user_id = \$1 AND COALESCE\(project_id, ''\) = \$2 AND run_id IS NULL ORDER BY created_at DESC LIMIT 1`).
		WithArgs("user-1", "p-1").
		WillReturnRows(sqlmock.NewRows([]string{"request"}).AddRow(newCfg))

	mock.ExpectQuery(`SELECT diagram_version_id, scenario_yaml, scenario_hash`).
		WithArgs("dv-1").
		WillReturnRows(sqlmock.NewRows([]string{"diagram_version_id", "scenario_yaml", "scenario_hash", "s3_path", "source", "source_hash", "created_at", "updated_at"}).
			AddRow("dv-1", "cached-yaml", "h", nil, "generated", oldGenHash, now, now))

	mock.ExpectQuery(`INSERT INTO simulation_scenario_cache`).
		WithArgs("dv-1", sqlmock.AnyArg(), sqlmock.AnyArg(), "", "generated", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"diagram_version_id", "scenario_yaml", "scenario_hash", "s3_path", "source", "source_hash", "created_at", "updated_at"}).
			AddRow("dv-1", "fresh", "h2", nil, "generated", "sh2", now, now))

	req := httptest.NewRequest(http.MethodGet, "/projects/p-1/diagram-versions/dv-1/scenario", nil)
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	yamlStr, _ := resp["scenario_yaml"].(string)
	require.Contains(t, yamlStr, "cores: 4")
	require.NotEqual(t, "cached-yaml", yamlStr)
	require.NoError(t, mock.ExpectationsWereMet())
}
