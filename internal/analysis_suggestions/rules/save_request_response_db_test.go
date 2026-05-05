package rules

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestSaveRequestResponseToDB_updatePreservesStoredSimulationNodes(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	userID := "user-1"
	projectID := "proj-1"
	runID := "run-1"
	design := DesignInput{PreferredVCPU: 99}
	simulation := SimulationInput{Nodes: 2}
	candidates := []Candidate{}
	response := []CandidateScore{}
	best := CandidateScore{}

	existingUUID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	storedReq := []byte(`{"design":{"preferred_vcpu":4,"preferred_memory_gb":16},"simulation":{"nodes":10},"candidates":[]}`)

	mock.ExpectQuery(`SELECT id\s+FROM request_responses`).
		WithArgs(userID, projectID, runID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(existingUUID))

	mock.ExpectQuery(`SELECT request FROM request_responses WHERE id = \$1`).
		WithArgs(existingUUID).
		WillReturnRows(sqlmock.NewRows([]string{"request"}).AddRow(storedReq))

	mock.ExpectQuery(`UPDATE request_responses`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), existingUUID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(existingUUID))

	id, err := saveRequestResponseToDB(ctx, db, userID, projectID, runID, design, simulation, candidates, response, best)
	require.NoError(t, err)
	require.Equal(t, existingUUID, id)

	require.NoError(t, mock.ExpectationsWereMet())
}
