package unit

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/domain"
	simrepo "github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/repository"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupMetricsRepo(t *testing.T) (*simrepo.MetricsTimeseriesRepository, sqlmock.Sqlmock, *sql.DB) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	repo := simrepo.NewMetricsTimeseriesRepository(db)
	return repo, mock, db
}

func TestMetricsTimeseriesRepository_InsertBatch(t *testing.T) {
	repo, mock, db := setupMetricsRepo(t)
	defer db.Close()

	t.Run("inserts batch of metrics successfully", func(t *testing.T) {
		points := []domain.MetricDataPoint{
			{
				RunID:       "run-123",
				Time:        time.Now(),
				TimestampMs: time.Now().UnixMilli(),
				MetricType:  "request_latency_ms",
				MetricValue: 125.5,
			},
			{
				RunID:       "run-123",
				Time:        time.Now(),
				TimestampMs: time.Now().UnixMilli(),
				MetricType:  "cpu_utilization",
				MetricValue: 65.2,
			},
		}

		mock.ExpectBegin()
		prep := mock.ExpectPrepare(`INSERT INTO simulation_metrics_timeseries`)
		prep.ExpectExec().
			WithArgs(
				"run-123", sqlmock.AnyArg(), sqlmock.AnyArg(), "request_latency_ms", 125.5,
				sql.NullString{}, sql.NullString{}, sqlmock.AnyArg(),
			).
			WillReturnResult(sqlmock.NewResult(1, 1))
		prep.ExpectExec().
			WithArgs(
				"run-123", sqlmock.AnyArg(), sqlmock.AnyArg(), "cpu_utilization", 65.2,
				sql.NullString{}, sql.NullString{}, sqlmock.AnyArg(),
			).
			WillReturnResult(sqlmock.NewResult(2, 1))
		mock.ExpectCommit()

		ctx := context.Background()
		err := repo.InsertBatch(ctx, points)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("handles empty batch", func(t *testing.T) {
		ctx := context.Background()
		err := repo.InsertBatch(ctx, []domain.MetricDataPoint{})
		require.NoError(t, err)
	})
}

func TestMetricsTimeseriesRepository_GetByRunID(t *testing.T) {
	repo, mock, db := setupMetricsRepo(t)
	defer db.Close()

	t.Run("gets all metrics for run", func(t *testing.T) {
		runID := "run-123"
		tagsJSON := `{"host":"server-1"}`

		mock.ExpectQuery(`SELECT id, run_id, time, timestamp_ms`).
			WithArgs(runID).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "run_id", "time", "timestamp_ms", "metric_type", "metric_value",
				"service_id", "node_id", "tags",
			}).
				AddRow(1, "run-123", time.Now(), int64(1234567890), "request_latency_ms", 125.5, nil, nil, tagsJSON).
				AddRow(2, "run-123", time.Now(), int64(1234567891), "cpu_utilization", 65.2, "service-1", nil, "{}"))

		ctx := context.Background()
		metrics, err := repo.GetByRunID(ctx, runID)
		require.NoError(t, err)
		assert.Len(t, metrics, 2)
		assert.Equal(t, "request_latency_ms", metrics[0].MetricType)
		assert.Equal(t, 125.5, metrics[0].MetricValue)
		assert.Equal(t, "cpu_utilization", metrics[1].MetricType)
		assert.Equal(t, 65.2, metrics[1].MetricValue)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestMetricsTimeseriesRepository_GetByRunIDAndType(t *testing.T) {
	repo, mock, db := setupMetricsRepo(t)
	defer db.Close()

	t.Run("gets metrics filtered by type", func(t *testing.T) {
		runID := "run-123"
		metricType := "request_latency_ms"

		mock.ExpectQuery(`SELECT id, run_id, time, timestamp_ms`).
			WithArgs(runID, metricType).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "run_id", "time", "timestamp_ms", "metric_type", "metric_value",
				"service_id", "node_id", "tags",
			}).
				AddRow(1, "run-123", time.Now(), int64(1234567890), "request_latency_ms", 125.5, nil, nil, "{}"))

		ctx := context.Background()
		metrics, err := repo.GetByRunIDAndType(ctx, runID, metricType)
		require.NoError(t, err)
		assert.Len(t, metrics, 1)
		assert.Equal(t, "request_latency_ms", metrics[0].MetricType)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestMetricsTimeseriesRepository_CountByRunID(t *testing.T) {
	repo, mock, db := setupMetricsRepo(t)
	defer db.Close()

	t.Run("counts metrics for run", func(t *testing.T) {
		runID := "run-123"

		mock.ExpectQuery(`SELECT COUNT\(\*\)`).
			WithArgs(runID).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(42))

		ctx := context.Background()
		count, err := repo.CountByRunID(ctx, runID)
		require.NoError(t, err)
		assert.Equal(t, int64(42), count)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
