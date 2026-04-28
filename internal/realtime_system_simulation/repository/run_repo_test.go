package repository

import (
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/domain"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRunRepoTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()

	mr, err := miniredis.Run()
	require.NoError(t, err)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = rdb.Close()
		mr.Close()
	})

	return rdb, mr
}

func TestRunRepositoryWithDB_ListByUserSurvivesRedisFlush(t *testing.T) {
	rdb, mr := newRunRepoTestRedis(t)
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewRunRepositoryWithDB(rdb, db)
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	run := &domain.SimulationRun{
		RunID:           "run-123",
		UserID:          "user-123",
		ProjectPublicID: "project-123",
		EngineRunID:     "engine-123",
		Status:          domain.StatusPending,
		CreatedAt:       now,
		UpdatedAt:       now,
		Metadata:        map[string]interface{}{"mode": "batch"},
	}

	mock.ExpectExec(`INSERT INTO simulation_runs`).
		WithArgs(
			run.RunID,
			run.UserID,
			run.ProjectPublicID,
			run.EngineRunID,
			run.Status,
			run.CreatedAt,
			run.UpdatedAt,
			run.CompletedAt,
			`{"mode":"batch"}`,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, repo.Create(run))
	mr.FlushAll()

	mock.ExpectQuery(`SELECT run_id\s+FROM simulation_runs\s+WHERE user_id = \$1\s+ORDER BY created_at DESC`).
		WithArgs("user-123").
		WillReturnRows(sqlmock.NewRows([]string{"run_id"}).AddRow("run-123"))

	runIDs, err := repo.ListByUserID("user-123")
	require.NoError(t, err)
	assert.Equal(t, []string{"run-123"}, runIDs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRunRepositoryWithDB_GetByRunIDFallsBackToRedis(t *testing.T) {
	rdb, _ := newRunRepoTestRedis(t)
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewRunRepositoryWithDB(rdb, db)
	run := &domain.SimulationRun{
		RunID:     "run-redis",
		UserID:    "user-redis",
		Status:    domain.StatusPending,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	redisOnlyRepo := NewRunRepository(rdb)
	require.NoError(t, redisOnlyRepo.Create(run))

	mock.ExpectQuery(`SELECT run_id`).
		WithArgs("run-redis", domain.StatusPending).
		WillReturnError(sql.ErrNoRows)

	got, err := repo.GetByRunID("run-redis")
	require.NoError(t, err)
	assert.Equal(t, "run-redis", got.RunID)
	assert.Equal(t, "user-redis", got.UserID)
	require.NoError(t, mock.ExpectationsWereMet())
}
