package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	simrepo "github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/repository"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestPostRegenerateDiagramVersionScenario_RequiresOverwriteWhenEdited(t *testing.T) {
	gin.SetMode(gin.TestMode)
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
		scenarioCacheRepo: simrepo.NewScenarioCacheRepository(db),
	}
	router := gin.New()
	router.POST("/projects/:project_id/diagram-versions/:diagram_version_id/scenario/regenerate", h.PostRegenerateDiagramVersionScenario)

	mock.ExpectQuery(`SELECT id FROM diagram_versions`).
		WithArgs("dv-1", "p-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("dv-1"))
	mock.ExpectQuery(`SELECT yaml_content FROM diagram_versions`).
		WithArgs("dv-1", "p-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"yaml_content"}).AddRow("services:\n  - id: g\n    type: api_gateway\n"))
	mock.ExpectQuery(`SELECT c.diagram_version_id, c.scenario_yaml, c.scenario_hash`).
		WithArgs("dv-1", "p-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"diagram_version_id", "scenario_yaml", "scenario_hash", "s3_path", "source", "source_hash", "created_at", "updated_at"}).
			AddRow("dv-1", "x", "h", nil, "edited", "sh", now, now))

	req := httptest.NewRequest(http.MethodPost, "/projects/p-1/diagram-versions/dv-1/scenario/regenerate", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusConflict, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPutDiagramVersionScenario_InvalidYAML(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, _, err := sqlmock.New()
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
	router.PUT("/projects/:project_id/diagram-versions/:diagram_version_id/scenario", h.PutDiagramVersionScenario)

	body := map[string]string{"scenario_yaml": "hosts: []"}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, "/projects/p-1/diagram-versions/dv-1/scenario", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}
