package http

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/domain"
	simrepo "github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/repository"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/service"
)

func seedRunInRedis(t *testing.T, rdb *redis.Client, runID, userID string) {
	t.Helper()
	run := &domain.SimulationRun{
		RunID:       runID,
		UserID:    userID,
		Status:    domain.StatusCompleted,
		EngineRunID: "engine-1",
	}
	data, err := json.Marshal(run)
	require.NoError(t, err)
	require.NoError(t, rdb.Set(context.Background(), "sim:run:"+runID, data, 0).Err())
}

func TestGetRunPersistedMetricsTimeSeries_FilteredByMetric(t *testing.T) {
	gin.SetMode(gin.TestMode)
	runID := "run-ts-1"
	userID := "user-1"

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	seedRunInRedis(t, rdb, runID, userID)
	runRepo := simrepo.NewRunRepository(rdb)
	simSvc := service.NewSimulationService(runRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().UTC()
	tagsJSON := []byte(`{"host":"host-1"}`)
	mock.ExpectQuery(`SELECT run_id, time, timestamp_ms, metric_type, metric_value, service_id, node_id, tags`).
		WithArgs(runID, "cpu_utilization").
		WillReturnRows(sqlmock.NewRows([]string{
			"run_id", "time", "timestamp_ms", "metric_type", "metric_value", "service_id", "node_id", "tags",
		}).AddRow(runID, now, now.UnixMilli(), "cpu_utilization", 0.0414, "", "", tagsJSON))

	h := &Handler{simService: simSvc, db: db}
	router := gin.New()
	router.GET("/runs/:id/metrics/timeseries", h.GetRunPersistedMetricsTimeSeries)

	req := httptest.NewRequest(http.MethodGet, "/runs/"+runID+"/metrics/timeseries?metric=cpu_utilization", nil)
	req.Header.Set("X-User-Id", userID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, runID, resp["run_id"])
	points := resp["points"].([]any)
	require.Len(t, points, 1)
	p0 := points[0].(map[string]any)
	assert.Equal(t, "cpu_utilization", p0["metric"])
	assert.Equal(t, "host-1", p0["host_id"])
	labels := p0["labels"].(map[string]any)
	assert.Equal(t, "host-1", labels["host"])
	assert.IsType(t, "", p0["timestamp"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetRunPersistedMetricsTimeSeries_UnfilteredMixedMetrics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	runID := "run-ts-2"
	userID := "user-1"

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	seedRunInRedis(t, rdb, runID, userID)
	runRepo := simrepo.NewRunRepository(rdb)
	simSvc := service.NewSimulationService(runRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().UTC()
	mock.ExpectQuery(`SELECT run_id, time, timestamp_ms, metric_type, metric_value, service_id, node_id, tags`).
		WithArgs(driver.Value(runID)).
		WillReturnRows(sqlmock.NewRows([]string{
			"run_id", "time", "timestamp_ms", "metric_type", "metric_value", "service_id", "node_id", "tags",
		}).
			AddRow(runID, now, now.UnixMilli(), "cpu_utilization", 0.1, "", "", []byte(`{}`)).
			AddRow(runID, now.Add(time.Second), now.Add(time.Second).UnixMilli(), "memory_utilization", 0.2, "", "", []byte(`{}`)))

	h := &Handler{simService: simSvc, db: db}
	router := gin.New()
	router.GET("/runs/:id/metrics/timeseries", h.GetRunPersistedMetricsTimeSeries)

	req := httptest.NewRequest(http.MethodGet, "/runs/"+runID+"/metrics/timeseries", nil)
	req.Header.Set("X-User-Id", userID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	points := resp["points"].([]any)
	require.Len(t, points, 2)
	m0 := points[0].(map[string]any)["metric"]
	m1 := points[1].(map[string]any)["metric"]
	assert.NotEqual(t, m0, m1)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetRunMetrics_NestedPointsIncludeLabelsHostAndFinalConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	runID := "run-m-1"
	userID := "user-1"

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	seedRunInRedis(t, rdb, runID, userID)
	runRepo := simrepo.NewRunRepository(rdb)
	simSvc := service.NewSimulationService(runRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	metricsJSON := []byte(`{"latency_ms":{"p95":10}}`)
	summaryJSON := []byte(`{}`)
	fcJSON := []byte(`{"placements":["p1"]}`)
	mock.ExpectQuery(`SELECT run_id, engine_run_id, metrics, summary_data, final_config`).
		WithArgs(runID).
		WillReturnRows(sqlmock.NewRows([]string{
			"run_id", "engine_run_id", "metrics", "summary_data", "final_config",
			"total_requests", "total_errors", "total_duration_ms",
		}).AddRow(
			runID, "engine-1", metricsJSON, summaryJSON, fcJSON,
			sql.NullInt64{}, sql.NullInt64{}, sql.NullInt64{},
		))

	now := time.Now().UTC()
	tagsJSON := []byte(`{"host":"host-1"}`)
	mock.ExpectQuery(`SELECT run_id, time, timestamp_ms, metric_type, metric_value, service_id, node_id, tags`).
		WithArgs(driver.Value(runID)).
		WillReturnRows(sqlmock.NewRows([]string{
			"run_id", "time", "timestamp_ms", "metric_type", "metric_value", "service_id", "node_id", "tags",
		}).AddRow(runID, now, now.UnixMilli(), "cpu_utilization", 0.5, "", "", tagsJSON))

	h := &Handler{simService: simSvc, db: db}
	router := gin.New()
	router.GET("/runs/:id/metrics", h.GetRunMetrics)

	req := httptest.NewRequest(http.MethodGet, "/runs/"+runID+"/metrics", nil)
	req.Header.Set("X-User-Id", userID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	summary := resp["summary"].(map[string]any)
	fc := summary["final_config"].(map[string]any)
	pl := fc["placements"].([]any)
	require.Len(t, pl, 1)

	ts := resp["timeseries"].([]any)
	require.Len(t, ts, 1)
	series0 := ts[0].(map[string]any)
	points := series0["points"].([]any)
	require.Len(t, points, 1)
	p0 := points[0].(map[string]any)
	labels := p0["labels"].(map[string]any)
	assert.Equal(t, "host-1", labels["host"])
	assert.Equal(t, "host-1", p0["host_id"])
	assert.Contains(t, p0, "tags")
	require.NoError(t, mock.ExpectationsWereMet())
}
