package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/domain"
	simrepo "github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/repository"
	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunSummaryResponse_JSON_finalConfigRoundTrip(t *testing.T) {
	const payload = `{"summary":{"run_id":"engine-run-1","total_requests":3,"final_config":{"placements":[{"id":"p1"}]}}}`
	var resp RunSummaryResponse
	require.NoError(t, json.Unmarshal([]byte(payload), &resp))
	require.NotNil(t, resp.Summary.FinalConfig)
	placements, ok := resp.Summary.FinalConfig["placements"].([]interface{})
	require.True(t, ok)
	require.Len(t, placements, 1)
	m, ok := placements[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "p1", m["id"])
}

// stubEngine implements EngineClient for StoreRunSummaryAndMetrics tests.
type stubEngine struct {
	summary *RunSummaryResponse
	metrics []MetricDataPointResponse
}

func (s *stubEngine) GetRunSummary(engineRunID string) (*RunSummaryResponse, error) {
	if s.summary == nil {
		return &RunSummaryResponse{}, nil
	}
	return s.summary, nil
}

func (s *stubEngine) GetRunMetrics(engineRunID string) ([]MetricDataPointResponse, error) {
	return s.metrics, nil
}

func TestStoreRunSummaryAndMetrics_PersistsFinalConfigWhenPresent(t *testing.T) {
	ctx := context.Background()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	runRepo := simrepo.NewRunRepository(rdb)

	run := &domain.SimulationRun{
		RunID:       "run-svc-1",
		UserID:    "user-1",
		EngineRunID: "eng-1",
		Status:    domain.StatusCompleted,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, runRepo.Create(run))

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	summaryRepo := simrepo.NewSummaryRepository(db)
	metricsRepo := simrepo.NewMetricsTimeseriesRepository(db)
	now := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)

	var summaryJSON RunSummaryResponse
	summaryJSON.Summary.RunID = "eng-1"
	summaryJSON.Summary.TotalRequests = 5
	summaryJSON.Summary.FinalConfig = map[string]interface{}{"from_engine": true}

	engine := &stubEngine{
		summary: &summaryJSON,
		metrics: []MetricDataPointResponse{
			{TimestampMs: 1000, MetricType: "request_count", MetricValue: 1},
		},
	}

	mock.ExpectQuery(`INSERT INTO simulation_summaries`).
		WithArgs(
			sqlmock.AnyArg(),
			"run-svc-1",
			"eng-1",
			sql.NullInt64{Int64: 5, Valid: true},
			sql.NullInt64{Valid: false},
			sql.NullInt64{Valid: false},
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			false,
			[]byte(`{"from_engine":true}`),
		).
		WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at"}).AddRow(now, now))

	mock.ExpectBegin()
	prep := mock.ExpectPrepare(`INSERT INTO simulation_metrics_timeseries`)
	prep.ExpectExec().
		WithArgs(
			"run-svc-1",
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			"request_count",
			float64(1),
			sql.NullString{},
			sql.NullString{},
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	svc := NewSimulationServiceWithPersistence(runRepo, summaryRepo, metricsRepo, engine)
	require.NoError(t, svc.StoreRunSummaryAndMetrics(ctx, "run-svc-1"))

	// INSERT expectation pins final_config JSON (see WithArgs above).
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreRunSummaryAndMetrics_NilFinalConfigFromEngine_PreservesViaRepo(t *testing.T) {
	ctx := context.Background()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	runRepo := simrepo.NewRunRepository(rdb)

	run := &domain.SimulationRun{
		RunID:       "run-svc-2",
		UserID:    "user-1",
		EngineRunID: "eng-2",
		Status:    domain.StatusCompleted,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, runRepo.Create(run))

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	summaryRepo := simrepo.NewSummaryRepository(db)
	metricsRepo := simrepo.NewMetricsTimeseriesRepository(db)
	now := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)

	// Seed DB row with final_config via first upsert (non-nil FinalConfig).
	first := &domain.SimulationSummary{
		ID:            "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		RunID:         "run-svc-2",
		EngineRunID:   "eng-2",
		TotalRequests: 1,
		Metrics:       map[string]interface{}{},
		SummaryData:   map[string]interface{}{},
		FinalConfig:   map[string]interface{}{"seed": "keep"},
	}
	mock.ExpectQuery(`INSERT INTO simulation_summaries`).
		WithArgs(
			first.ID,
			first.RunID,
			first.EngineRunID,
			sql.NullInt64{Int64: 1, Valid: true},
			sql.NullInt64{Valid: false},
			sql.NullInt64{Valid: false},
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			false,
			[]byte(`{"seed":"keep"}`),
		).
		WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at"}).AddRow(now, now))
	require.NoError(t, summaryRepo.CreateOrUpdate(first))

	// Engine returns summary without final_config → nil FinalConfig → preserve in second upsert.
	engine := &stubEngine{
		summary: &RunSummaryResponse{}, // Summary.FinalConfig nil
		metrics: []MetricDataPointResponse{
			{TimestampMs: 2000, MetricType: "request_count", MetricValue: 2},
		},
	}

	mock.ExpectQuery(`INSERT INTO simulation_summaries`).
		WithArgs(
			sqlmock.AnyArg(),
			"run-svc-2",
			"",
			sql.NullInt64{Valid: false},
			sql.NullInt64{Valid: false},
			sql.NullInt64{Valid: false},
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			true,
			[]byte("{}"),
		).
		WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at"}).AddRow(now, now))

	mock.ExpectBegin()
	prep := mock.ExpectPrepare(`INSERT INTO simulation_metrics_timeseries`)
	prep.ExpectExec().
		WithArgs(
			"run-svc-2",
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			"request_count",
			float64(2),
			sql.NullString{},
			sql.NullString{},
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	svc := NewSimulationServiceWithPersistence(runRepo, summaryRepo, metricsRepo, engine)
	require.NoError(t, svc.StoreRunSummaryAndMetrics(ctx, "run-svc-2"))

	// Second upsert used preserve=true (nil FinalConfig from engine); repository tests assert DB behavior.
	require.NoError(t, mock.ExpectationsWereMet())
}
