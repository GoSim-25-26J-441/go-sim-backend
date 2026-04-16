package unit

import (
	"database/sql"
	"testing"
	"time"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/domain"
	simrepo "github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/repository"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupSummaryRepo(t *testing.T) (*simrepo.SummaryRepository, sqlmock.Sqlmock, *sql.DB) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	repo := simrepo.NewSummaryRepository(db)
	return repo, mock, db
}

func TestSummaryRepository_CreateOrUpdate(t *testing.T) {
	repo, mock, db := setupSummaryRepo(t)
	defer db.Close()

	t.Run("creates new summary successfully", func(t *testing.T) {
		summary := &domain.SimulationSummary{
			RunID:         "run-123",
			EngineRunID:   "engine-123",
			TotalRequests: 1000,
			TotalErrors:   10,
			TotalDuration: 120000,
			Metrics: map[string]interface{}{
				"request_latency": map[string]interface{}{
					"avg": 125.5,
					"p95": 250.0,
				},
			},
			SummaryData: map[string]interface{}{
				"scenario": "test-scenario",
			},
		}

		mock.ExpectQuery(`INSERT INTO simulation_summaries`).
			WithArgs(
				sqlmock.AnyArg(), // id (UUID)
				"run-123",
				"engine-123",
				sqlmock.AnyArg(), // total_requests
				sqlmock.AnyArg(), // total_errors
				sqlmock.AnyArg(), // total_duration_ms
				sqlmock.AnyArg(), // metrics JSONB
				sqlmock.AnyArg(), // summary_data JSONB
			).
			WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at"}).
				AddRow(time.Now(), time.Now()))

		err := repo.CreateOrUpdate(summary)
		require.NoError(t, err)
		assert.NotEmpty(t, summary.ID)
		assert.False(t, summary.CreatedAt.IsZero())
		assert.False(t, summary.UpdatedAt.IsZero())

		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("updates existing summary", func(t *testing.T) {
		summary := &domain.SimulationSummary{
			ID:            "existing-uuid",
			RunID:         "run-123",
			EngineRunID:   "engine-123",
			TotalRequests: 2000,
			TotalErrors:   20,
		}

		mock.ExpectQuery(`INSERT INTO simulation_summaries`).
			WithArgs(
				"existing-uuid",
				"run-123",
				"engine-123",
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
			).
			WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at"}).
				AddRow(time.Now(), time.Now()))

		err := repo.CreateOrUpdate(summary)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestSummaryRepository_GetByRunID(t *testing.T) {
	repo, mock, db := setupSummaryRepo(t)
	defer db.Close()

	t.Run("gets summary successfully", func(t *testing.T) {
		runID := "run-123"
		metricsJSON := `{"request_latency":{"avg":125.5,"p95":250.0}}`
		summaryDataJSON := `{"scenario":"test-scenario"}`

		mock.ExpectQuery(`SELECT id, run_id, engine_run_id`).
			WithArgs(runID).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "run_id", "engine_run_id", "total_requests", "total_errors",
				"total_duration_ms", "metrics", "summary_data", "created_at", "updated_at",
			}).
				AddRow(
					"uuid-123",
					"run-123",
					"engine-123",
					1000,
					10,
					120000,
					metricsJSON,
					summaryDataJSON,
					time.Now(),
					time.Now(),
				))

		summary, err := repo.GetByRunID(runID)
		require.NoError(t, err)
		assert.Equal(t, "run-123", summary.RunID)
		assert.Equal(t, "engine-123", summary.EngineRunID)
		assert.Equal(t, int64(1000), summary.TotalRequests)
		assert.Equal(t, int64(10), summary.TotalErrors)
		assert.NotNil(t, summary.Metrics)
		assert.NotNil(t, summary.SummaryData)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns error for non-existent summary", func(t *testing.T) {
		mock.ExpectQuery(`SELECT id, run_id, engine_run_id`).
			WithArgs("non-existent").
			WillReturnError(sql.ErrNoRows)

		_, err := repo.GetByRunID("non-existent")
		assert.Error(t, err)
		assert.Equal(t, domain.ErrRunNotFound, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestSummaryRepository_Exists(t *testing.T) {
	repo, mock, db := setupSummaryRepo(t)
	defer db.Close()

	t.Run("returns true when summary exists", func(t *testing.T) {
		mock.ExpectQuery(`SELECT EXISTS`).
			WithArgs("run-123").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

		exists, err := repo.Exists("run-123")
		require.NoError(t, err)
		assert.True(t, exists)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns false when summary does not exist", func(t *testing.T) {
		mock.ExpectQuery(`SELECT EXISTS`).
			WithArgs("non-existent").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

		exists, err := repo.Exists("non-existent")
		require.NoError(t, err)
		assert.False(t, exists)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
