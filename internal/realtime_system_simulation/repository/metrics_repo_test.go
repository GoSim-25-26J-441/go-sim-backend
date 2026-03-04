package repository

import (
	"context"
	"database/sql/driver"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
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
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = repo.UpsertSummary(context.Background(), &SummaryUpsertParams{
		RunID:       runID,
		EngineRunID: engineRunID,
		Metrics: map[string]any{
			"request_latency_ms": map[string]any{
				"p95": 120.0,
			},
		},
		SummaryData: map[string]any{
			"note": "test summary",
		},
	})
	require.NoError(t, err)
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

