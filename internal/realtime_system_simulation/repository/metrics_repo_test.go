package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricsRepository_UpsertSummary_CreatesSimulationRunsAndSummary(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewMetricsRepository(db)

	runID := "run-123"
	engineRunID := "engine-123"

	// Expect ensuring simulation_runs row
	mock.ExpectExec(`INSERT INTO simulation_runs`).
		WithArgs(driver.Value(runID)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Expect upsert into simulation_summaries
	mock.ExpectExec(`INSERT INTO simulation_summaries`).
		WithArgs(
			driver.Value(runID),
			driver.Value(engineRunID),
			sqlmock.AnyArg(), // metrics JSON
			sqlmock.AnyArg(), // summary_data JSON
			driver.Value(""), // scenario_yaml
			nil,                // total_duration_ms
			sqlmock.AnyArg(),   // final_config JSON
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = repo.UpsertSummary(context.Background(), &SummaryUpsertParams{
		RunID:        runID,
		EngineRunID:  engineRunID,
		Metrics: map[string]any{
			"request_latency_ms": map[string]any{
				"p95": 120.0,
			},
		},
		SummaryData:  map[string]any{"note": "test summary"},
		ScenarioYAML: "",
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMetricsRepository_UpsertSummary_NilFinalConfigStoredAsObject(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewMetricsRepository(db)
	runID := "run-fc-nil"
	engineRunID := "engine-fc"

	mock.ExpectExec(`INSERT INTO simulation_runs`).
		WithArgs(driver.Value(runID)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO simulation_summaries`).
		WithArgs(
			driver.Value(runID),
			driver.Value(engineRunID),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			driver.Value(""),
			nil,
			driver.Value("{}"),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = repo.UpsertSummary(context.Background(), &SummaryUpsertParams{
		RunID:       runID,
		EngineRunID: engineRunID,
		Metrics:     map[string]any{"k": 1},
		SummaryData: map[string]any{},
		FinalConfig: nil,
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMetricsRepository_UpsertSummary_PersistsTotalDurationMs(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewMetricsRepository(db)
	runID := "run-456"
	engineRunID := "engine-456"
	totalDurationMs := int64(5000)

	mock.ExpectExec(`INSERT INTO simulation_runs`).
		WithArgs(driver.Value(runID)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO simulation_summaries`).
		WithArgs(
			driver.Value(runID),
			driver.Value(engineRunID),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			driver.Value(""),
			driver.Value(totalDurationMs),
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = repo.UpsertSummary(context.Background(), &SummaryUpsertParams{
		RunID:           runID,
		EngineRunID:     engineRunID,
		Metrics:         map[string]any{"n": 1},
		SummaryData:     map[string]any{},
		ScenarioYAML:    "",
		TotalDurationMs: &totalDurationMs,
	})
	require.NoError(t, err)

	metricsJSON, _ := json.Marshal(map[string]any{"n": 1})
	summaryJSON, _ := json.Marshal(map[string]any{})
	fcJSON := []byte(`{}`)
	mock.ExpectQuery(`SELECT run_id, engine_run_id, metrics, summary_data, final_config`).
		WithArgs(runID).
		WillReturnRows(sqlmock.NewRows([]string{
			"run_id", "engine_run_id", "metrics", "summary_data", "final_config",
			"total_requests", "total_errors", "total_duration_ms",
		}).AddRow(
			runID, engineRunID, metricsJSON, summaryJSON, fcJSON,
			sql.NullInt64{}, sql.NullInt64{}, sql.NullInt64{Int64: totalDurationMs, Valid: true},
		))

	summary, err := repo.GetSummaryByRunID(context.Background(), runID)
	require.NoError(t, err)
	require.NotNil(t, summary)
	assert.True(t, summary.TotalDurationMs.Valid)
	assert.Equal(t, totalDurationMs, summary.TotalDurationMs.Int64)
	require.NotNil(t, summary.FinalConfig)
	assert.Empty(t, summary.FinalConfig)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMetricsRepository_GetSummaryByRunID_FinalConfigRoundTrip(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewMetricsRepository(db)
	runID := "run-fc-rt"
	engineRunID := "eng-fc"
	metricsJSON, _ := json.Marshal(map[string]any{"m": 1})
	summaryJSON, _ := json.Marshal(map[string]any{})
	fcWant := map[string]any{"placements": []any{"a", "b"}}
	fcJSON, err := json.Marshal(fcWant)
	require.NoError(t, err)

	mock.ExpectQuery(`SELECT run_id, engine_run_id, metrics, summary_data, final_config`).
		WithArgs(runID).
		WillReturnRows(sqlmock.NewRows([]string{
			"run_id", "engine_run_id", "metrics", "summary_data", "final_config",
			"total_requests", "total_errors", "total_duration_ms",
		}).AddRow(
			runID, engineRunID, metricsJSON, summaryJSON, fcJSON,
			sql.NullInt64{}, sql.NullInt64{}, sql.NullInt64{},
		))

	summary, err := repo.GetSummaryByRunID(context.Background(), runID)
	require.NoError(t, err)
	require.NotNil(t, summary)
	pl, ok := summary.FinalConfig["placements"].([]any)
	require.True(t, ok)
	assert.Len(t, pl, 2)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMetricsRepository_ListTimeSeriesByRunIDAndMetric_FiltersByMetric(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewMetricsRepository(db)
	runID := "run-789"
	metric := "cpu_utilization"
	now := time.Now().UTC()
	tagsJSON := []byte(`{"host":"host-1"}`)

	mock.ExpectQuery(`SELECT run_id, time, timestamp_ms, metric_type, metric_value, service_id, node_id, tags`).
		WithArgs(runID, metric).
		WillReturnRows(sqlmock.NewRows([]string{
			"run_id", "time", "timestamp_ms", "metric_type", "metric_value", "service_id", "node_id", "tags",
		}).AddRow(runID, now, now.UnixMilli(), metric, 0.5, "", "host-1", tagsJSON))

	points, err := repo.ListTimeSeriesByRunIDAndMetric(context.Background(), runID, metric)
	require.NoError(t, err)
	require.Len(t, points, 1)
	assert.Equal(t, metric, points[0].MetricType)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMetricsRepository_InsertTimeSeries_InsertsBatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewMetricsRepository(db)

	now := time.Now().UTC()
	points := []TimeSeriesPoint{
		{
			RunID:       "run-123",
			Time:        now,
			TimestampMs: now.UnixMilli(),
			MetricType:  "cpu_utilization",
			MetricValue: 0.75,
			ServiceID:   "svc1",
			NodeID:      "host-1",
			Tags: map[string]any{
				"region": "us-east-1",
			},
		},
		{
			RunID:       "run-123",
			Time:        now.Add(time.Second),
			TimestampMs: now.Add(time.Second).UnixMilli(),
			MetricType:  "cpu_utilization",
			MetricValue: 0.80,
			ServiceID:   "svc1",
			NodeID:      "host-1",
			Tags: map[string]any{
				"region": "us-east-1",
			},
		},
	}

	mock.ExpectExec(`INSERT INTO simulation_runs`).
		WithArgs(driver.Value("run-123")).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectBegin()
	mock.ExpectPrepare(`INSERT INTO simulation_metrics_timeseries`)
	mock.ExpectExec(`INSERT INTO simulation_metrics_timeseries`).
		WithArgs(
			driver.Value(points[0].RunID),
			sqlmock.AnyArg(), // time
			driver.Value(points[0].TimestampMs),
			driver.Value(points[0].MetricType),
			driver.Value(points[0].MetricValue),
			driver.Value(points[0].ServiceID),
			driver.Value(points[0].NodeID),
			sqlmock.AnyArg(), // tags JSON
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO simulation_metrics_timeseries`).
		WithArgs(
			driver.Value(points[1].RunID),
			sqlmock.AnyArg(), // time
			driver.Value(points[1].TimestampMs),
			driver.Value(points[1].MetricType),
			driver.Value(points[1].MetricValue),
			driver.Value(points[1].ServiceID),
			driver.Value(points[1].NodeID),
			sqlmock.AnyArg(), // tags JSON
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err = repo.InsertTimeSeries(context.Background(), points)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
