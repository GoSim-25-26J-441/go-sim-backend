package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestLoadLatestScenarioHostConfig(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	raw := []byte(`{"design":{"preferred_vcpu":4,"preferred_memory_gb":16},"simulation":{"nodes":3}}`)
	mock.ExpectQuery(`SELECT request FROM request_responses WHERE user_id = \$1 AND COALESCE\(project_id, ''\) = \$2 AND run_id IS NULL ORDER BY created_at DESC LIMIT 1`).
		WithArgs("user-1", "proj-1").
		WillReturnRows(sqlmock.NewRows([]string{"request"}).AddRow(raw))

	cfg, ok, err := LoadLatestScenarioHostConfig(context.Background(), db, "user-1", "proj-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, 3, cfg.Nodes)
	require.Equal(t, 4, cfg.Cores)
	require.InDelta(t, 16.0, cfg.MemoryGB, 1e-9)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadLatestScenarioHostConfig_noRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT request FROM request_responses WHERE user_id = \$1 AND COALESCE\(project_id, ''\) = \$2 AND run_id IS NULL ORDER BY created_at DESC LIMIT 1`).
		WithArgs("user-1", "proj-1").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`SELECT request FROM request_responses WHERE user_id = \$1 AND COALESCE\(project_id, ''\) = \$2 ORDER BY created_at DESC LIMIT 1`).
		WithArgs("user-1", "proj-1").
		WillReturnError(sql.ErrNoRows)

	_, ok, err := LoadLatestScenarioHostConfig(context.Background(), db, "user-1", "proj-1")
	require.NoError(t, err)
	require.False(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadLatestScenarioHostConfig_designOnlyInvalidFallsBackToLatestAny(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	invalid := []byte(`{"design":{"preferred_vcpu":0},"simulation":{"nodes":1}}`)
	valid := []byte(`{"design":{"preferred_vcpu":4,"preferred_memory_gb":16},"simulation":{"nodes":3}}`)
	mock.ExpectQuery(`SELECT request FROM request_responses WHERE user_id = \$1 AND COALESCE\(project_id, ''\) = \$2 AND run_id IS NULL ORDER BY created_at DESC LIMIT 1`).
		WithArgs("user-1", "proj-1").
		WillReturnRows(sqlmock.NewRows([]string{"request"}).AddRow(invalid))
	mock.ExpectQuery(`SELECT request FROM request_responses WHERE user_id = \$1 AND COALESCE\(project_id, ''\) = \$2 ORDER BY created_at DESC LIMIT 1`).
		WithArgs("user-1", "proj-1").
		WillReturnRows(sqlmock.NewRows([]string{"request"}).AddRow(valid))

	cfg, ok, err := LoadLatestScenarioHostConfig(context.Background(), db, "user-1", "proj-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, 3, cfg.Nodes)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadLatestScenarioHostConfig_designOnlyQueryDBErrorNoFallback(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	dbErr := errors.New("connection refused")
	mock.ExpectQuery(`SELECT request FROM request_responses WHERE user_id = \$1 AND COALESCE\(project_id, ''\) = \$2 AND run_id IS NULL ORDER BY created_at DESC LIMIT 1`).
		WithArgs("user-1", "proj-1").
		WillReturnError(dbErr)

	_, _, err = LoadLatestScenarioHostConfig(context.Background(), db, "user-1", "proj-1")
	require.ErrorIs(t, err, dbErr)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadLatestScenarioHostConfig_secondQueryDBError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	dbErr := errors.New("timeout")
	mock.ExpectQuery(`SELECT request FROM request_responses WHERE user_id = \$1 AND COALESCE\(project_id, ''\) = \$2 AND run_id IS NULL ORDER BY created_at DESC LIMIT 1`).
		WithArgs("user-1", "proj-1").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`SELECT request FROM request_responses WHERE user_id = \$1 AND COALESCE\(project_id, ''\) = \$2 ORDER BY created_at DESC LIMIT 1`).
		WithArgs("user-1", "proj-1").
		WillReturnError(dbErr)

	_, _, err = LoadLatestScenarioHostConfig(context.Background(), db, "user-1", "proj-1")
	require.ErrorIs(t, err, dbErr)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadLatestScenarioHostConfig_fallbackLatestAny(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	raw := []byte(`{"design":{"preferred_vcpu":2,"preferred_memory_gb":8},"simulation":{"nodes":2}}`)
	mock.ExpectQuery(`SELECT request FROM request_responses WHERE user_id = \$1 AND COALESCE\(project_id, ''\) = \$2 AND run_id IS NULL ORDER BY created_at DESC LIMIT 1`).
		WithArgs("user-1", "proj-1").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`SELECT request FROM request_responses WHERE user_id = \$1 AND COALESCE\(project_id, ''\) = \$2 ORDER BY created_at DESC LIMIT 1`).
		WithArgs("user-1", "proj-1").
		WillReturnRows(sqlmock.NewRows([]string{"request"}).AddRow(raw))

	cfg, ok, err := LoadLatestScenarioHostConfig(context.Background(), db, "user-1", "proj-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, 2, cfg.Nodes)
	require.Equal(t, 2, cfg.Cores)
	require.NoError(t, mock.ExpectationsWereMet())
}

