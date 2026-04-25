package repository

import (
	"database/sql"
	"testing"
	"time"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/domain"
	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSummaryRepository_CreateOrUpdate_PersistsFinalConfig(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewSummaryRepository(db)
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	summary := &domain.SimulationSummary{
		ID:            "11111111-1111-1111-1111-111111111111",
		RunID:         "run-1",
		EngineRunID:   "engine-1",
		TotalRequests: 10,
		TotalErrors:   1,
		TotalDuration: 100,
		Metrics:       map[string]interface{}{"latency_ms": map[string]interface{}{"p95": 42.0}},
		SummaryData:   map[string]interface{}{"k": "v"},
		FinalConfig:   map[string]interface{}{"placements": []interface{}{"a", "b"}},
	}

	mock.ExpectQuery(`INSERT INTO simulation_summaries`).
		WithArgs(
			summary.ID,
			summary.RunID,
			summary.EngineRunID,
			sql.NullInt64{Int64: 10, Valid: true},
			sql.NullInt64{Int64: 1, Valid: true},
			sql.NullInt64{Int64: 100, Valid: true},
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			false,
			[]byte(`{"placements":["a","b"]}`),
		).
		WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at"}).AddRow(now, now))

	require.NoError(t, repo.CreateOrUpdate(summary))
	assert.Equal(t, now, summary.CreatedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSummaryRepository_GetByRunID_ReturnsFinalConfig(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewSummaryRepository(db)
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mock.ExpectQuery(`SELECT id, run_id, engine_run_id, total_requests, total_errors`).
		WithArgs("run-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "run_id", "engine_run_id", "total_requests", "total_errors",
			"total_duration_ms", "metrics", "summary_data", "coalesce", "created_at", "updated_at",
		}).AddRow(
			"11111111-1111-1111-1111-111111111111",
			"run-1",
			"engine-1",
			sql.NullInt64{Int64: 5, Valid: true},
			sql.NullInt64{Int64: 0, Valid: false},
			sql.NullInt64{Int64: 50, Valid: true},
			[]byte(`{}`),
			[]byte(`{}`),
			[]byte(`{"zones":["z1"]}`),
			now,
			now,
		))

	got, err := repo.GetByRunID("run-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "run-1", got.RunID)
	require.Contains(t, got.FinalConfig, "zones")
	assert.Equal(t, []interface{}{"z1"}, got.FinalConfig["zones"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSummaryRepository_CreateOrUpdate_NilFinalConfig_PreservesExistingOnConflict(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewSummaryRepository(db)
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	first := &domain.SimulationSummary{
		ID:            "11111111-1111-1111-1111-111111111111",
		RunID:         "run-preserve",
		EngineRunID:   "engine-p",
		TotalRequests: 1,
		Metrics:       map[string]interface{}{},
		SummaryData:   map[string]interface{}{},
		FinalConfig:   map[string]interface{}{"keep": true},
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
			[]byte(`{"keep":true}`),
		).
		WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at"}).AddRow(now, now))
	require.NoError(t, repo.CreateOrUpdate(first))

	second := &domain.SimulationSummary{
		ID:            "22222222-2222-2222-2222-222222222222",
		RunID:         "run-preserve",
		EngineRunID:   "engine-p",
		TotalRequests: 2,
		Metrics:       map[string]interface{}{"x": 1.0},
		SummaryData:   map[string]interface{}{"y": 2.0},
		FinalConfig:   nil,
	}
	mock.ExpectQuery(`INSERT INTO simulation_summaries`).
		WithArgs(
			second.ID,
			second.RunID,
			second.EngineRunID,
			sql.NullInt64{Int64: 2, Valid: true},
			sql.NullInt64{Valid: false},
			sql.NullInt64{Valid: false},
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			true,
			[]byte("{}"),
		).
		WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at"}).AddRow(now, now))
	require.NoError(t, repo.CreateOrUpdate(second))

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSummaryRepository_GetByRunID_EmptyFinalConfigJSON(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewSummaryRepository(db)
	now := time.Now()

	mock.ExpectQuery(`SELECT id, run_id, engine_run_id, total_requests, total_errors`).
		WithArgs("run-empty-fc").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "run_id", "engine_run_id", "total_requests", "total_errors",
			"total_duration_ms", "metrics", "summary_data", "coalesce", "created_at", "updated_at",
		}).AddRow(
			"11111111-1111-1111-1111-111111111111",
			"run-empty-fc",
			"e1",
			sql.NullInt64{Valid: false},
			sql.NullInt64{Valid: false},
			sql.NullInt64{Valid: false},
			[]byte(`{}`),
			[]byte(`{}`),
			[]byte(`{}`),
			now,
			now,
		))

	got, err := repo.GetByRunID("run-empty-fc")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.NotNil(t, got.FinalConfig)
	assert.Len(t, got.FinalConfig, 0)
	require.NoError(t, mock.ExpectationsWereMet())
}
