package repository

import (
	"context"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestScenarioCacheRepository_UpsertAndGet(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewScenarioCacheRepository(db)
	now := time.Now()

	mock.ExpectQuery(`SELECT diagram_version_id, scenario_yaml, scenario_hash`).
		WithArgs("dv-1").
		WillReturnRows(sqlmock.NewRows([]string{"diagram_version_id", "scenario_yaml", "scenario_hash", "s3_path", "source", "source_hash", "created_at", "updated_at"}))
	mock.ExpectQuery(`INSERT INTO simulation_scenario_cache`).
		WithArgs("dv-1", "services: []", sqlmock.AnyArg(), "", "request", nil).
		WillReturnRows(sqlmock.NewRows([]string{"diagram_version_id", "scenario_yaml", "scenario_hash", "s3_path", "source", "source_hash", "created_at", "updated_at"}).
			AddRow("dv-1", "services: []", "hash1", nil, "request", nil, now, now))

	created, err := repo.UpsertScenarioForDiagramVersion(context.Background(), "dv-1", "services: []", "request", "", nil, false)
	require.NoError(t, err)
	require.Equal(t, "dv-1", created.DiagramVersionID)

	mock.ExpectQuery(`SELECT c.diagram_version_id, c.scenario_yaml, c.scenario_hash`).
		WithArgs("dv-1", "project-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"diagram_version_id", "scenario_yaml", "scenario_hash", "s3_path", "source", "source_hash", "created_at", "updated_at"}).
			AddRow("dv-1", "services: []", "hash1", nil, "request", nil, now, now))
	got, err := repo.GetScenarioForDiagramVersion(context.Background(), "user-1", "project-1", "dv-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "services: []", got.ScenarioYAML)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestScenarioCacheRepository_UpsertConflictAndOverwrite(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewScenarioCacheRepository(db)
	now := time.Now()

	mock.ExpectQuery(`SELECT diagram_version_id, scenario_yaml, scenario_hash`).
		WithArgs("dv-1").
		WillReturnRows(sqlmock.NewRows([]string{"diagram_version_id", "scenario_yaml", "scenario_hash", "s3_path", "source", "source_hash", "created_at", "updated_at"}).
			AddRow("dv-1", "old", "old-hash", nil, "request", nil, now, now))
	_, err = repo.UpsertScenarioForDiagramVersion(context.Background(), "dv-1", "new", "request", "", nil, false)
	require.ErrorIs(t, err, ErrScenarioCacheConflict)

	mock.ExpectQuery(`SELECT diagram_version_id, scenario_yaml, scenario_hash`).
		WithArgs("dv-1").
		WillReturnRows(sqlmock.NewRows([]string{"diagram_version_id", "scenario_yaml", "scenario_hash", "s3_path", "source", "source_hash", "created_at", "updated_at"}).
			AddRow("dv-1", "old", "old-hash", nil, "request", nil, now, now))
	mock.ExpectQuery(`INSERT INTO simulation_scenario_cache`).
		WithArgs("dv-1", "new", sqlmock.AnyArg(), "", "request", nil).
		WillReturnRows(sqlmock.NewRows([]string{"diagram_version_id", "scenario_yaml", "scenario_hash", "s3_path", "source", "source_hash", "created_at", "updated_at"}).
			AddRow("dv-1", "new", "new-hash", nil, "request", nil, now, now))
	_, err = repo.UpsertScenarioForDiagramVersion(context.Background(), "dv-1", "new", "request", "", nil, true)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestScenarioCacheRepository_IdempotentAndScoped(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewScenarioCacheRepository(db)
	now := time.Now()
	hash := hashScenarioYAML("same")

	mock.ExpectQuery(`SELECT diagram_version_id, scenario_yaml, scenario_hash`).
		WithArgs("dv-1").
		WillReturnRows(sqlmock.NewRows([]string{"diagram_version_id", "scenario_yaml", "scenario_hash", "s3_path", "source", "source_hash", "created_at", "updated_at"}).
			AddRow("dv-1", "same", hash, nil, "request", nil, now, now))
	_, err = repo.UpsertScenarioForDiagramVersion(context.Background(), "dv-1", "same", "request", "", nil, false)
	require.NoError(t, err)

	mock.ExpectQuery(`SELECT c.diagram_version_id, c.scenario_yaml, c.scenario_hash`).
		WithArgs("dv-1", "project-2", "user-2").
		WillReturnRows(sqlmock.NewRows([]string{"diagram_version_id", "scenario_yaml", "scenario_hash", "s3_path", "source", "source_hash", "created_at", "updated_at"}))
	got, err := repo.GetScenarioForDiagramVersion(context.Background(), "user-2", "project-2", "dv-1")
	require.NoError(t, err)
	require.Nil(t, got)
	require.NoError(t, mock.ExpectationsWereMet())
}
