package http

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/domain"
	simrepo "github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/repository"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/service"
)

// fakeObjectStorage is a simple in-memory implementation of ObjectStorage for tests.
type fakeObjectStorage struct {
	objects map[string][]byte
}

func (f *fakeObjectStorage) PutObject(_ context.Context, key string, data []byte) error {
	if f.objects == nil {
		f.objects = make(map[string][]byte)
	}
	f.objects[key] = data
	return nil
}

func (f *fakeObjectStorage) GetObject(_ context.Context, key string) ([]byte, error) {
	if f.objects == nil {
		return nil, nil
	}
	return f.objects[key], nil
}

// TestHandler_GetRunCandidates_Unified tests the unified candidates endpoint returns
// candidates array plus best_candidate_id and best_candidate when summary and S3 are available.
func TestHandler_GetRunCandidates_Unified(t *testing.T) {
	gin.SetMode(gin.TestMode)

	runID := "run-123"
	userID := "user-456"
	s3Key := "simulation/run-123/best_scenario.yaml"
	bestCandidateID := "best-1"

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	run := &domain.SimulationRun{
		RunID:            runID,
		UserID:           userID,
		ProjectPublicID:  "proj-1",
		Status:           domain.StatusCompleted,
		EngineRunID:      "engine-123",
	}
	runData, err := json.Marshal(run)
	require.NoError(t, err)
	err = rdb.Set(context.Background(), "sim:run:"+runID, runData, 0).Err()
	require.NoError(t, err)

	runRepo := simrepo.NewRunRepository(rdb)
	simSvc := service.NewSimulationService(runRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Candidates list query (ListByRunID)
	mock.ExpectQuery(`SELECT id, user_id, project_public_id, run_id, candidate_id`).
		WithArgs(driver.Value(runID)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "project_public_id", "run_id", "candidate_id",
			"spec", "metrics", "sim_workload", "source", "s3_path",
		}).AddRow(
			"uuid-1", userID, "proj-1", runID, bestCandidateID,
			[]byte("{}"), []byte("{}"), []byte("{}"), "export", s3Key,
		))

	// Best candidate path query
	mock.ExpectQuery(`SELECT best_candidate_s3_path FROM simulation_summaries WHERE run_id = \$1`).
		WithArgs(driver.Value(runID)).
		WillReturnRows(sqlmock.NewRows([]string{"best_candidate_s3_path"}).AddRow(s3Key))

	yamlContent := `
hosts:
  - id: host-1
    cores: 4
  - id: host-2
    cores: 4
services:
  - id: svc1
    replicas: 3
    cpu_cores: 1
    memory_mb: 512
`
	storage := &fakeObjectStorage{
		objects: map[string][]byte{s3Key: []byte(yamlContent)},
	}

	h := &Handler{
		simService: simSvc,
		db:         db,
		s3Client:   storage,
	}

	router := gin.New()
	router.GET("/runs/:id/candidates", h.GetRunCandidates)

	req := httptest.NewRequest(http.MethodGet, "/runs/"+runID+"/candidates", nil)
	req.Header.Set("X-User-Id", userID)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var resp struct {
		UserID           string          `json:"user_id"`
		ProjectID        string          `json:"project_id"`
		RunID            string          `json:"run_id"`
		BestCandidateID  string          `json:"best_candidate_id"`
		BestCandidate    json.RawMessage `json:"best_candidate"`
		Candidates       []struct {
			ID   string `json:"id"`
			Spec map[string]interface{} `json:"spec"`
		} `json:"candidates"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, userID, resp.UserID)
	assert.Equal(t, runID, resp.RunID)
	assert.Equal(t, bestCandidateID, resp.BestCandidateID)
	require.Len(t, resp.Candidates, 1)
	assert.Equal(t, bestCandidateID, resp.Candidates[0].ID)

	require.True(t, len(resp.BestCandidate) > 0, "best_candidate should be present")
	var bc struct {
		S3Path   string `json:"s3_path"`
		Hosts    []struct {
			HostID   string  `json:"host_id"`
			CPUCores int     `json:"cpu_cores"`
			MemoryGB float64 `json:"memory_gb"`
		} `json:"hosts"`
		Services []struct {
			ServiceID string  `json:"service_id"`
			Replicas  int     `json:"replicas"`
			CPUCores  float64 `json:"cpu_cores"`
			MemoryMB  float64 `json:"memory_mb"`
		} `json:"services"`
	}
	err = json.Unmarshal(resp.BestCandidate, &bc)
	require.NoError(t, err)
	assert.Equal(t, s3Key, bc.S3Path)
	require.Len(t, bc.Hosts, 2)
	assert.Equal(t, "host-1", bc.Hosts[0].HostID)
	assert.Equal(t, 4, bc.Hosts[0].CPUCores)
	require.Len(t, bc.Services, 1)
	assert.Equal(t, "svc1", bc.Services[0].ServiceID)
	assert.Equal(t, 3, bc.Services[0].Replicas)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUploadBestScenarioToS3_StoresObjectAndDBRecord(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Mock simulation engine export endpoint
	engineRunID := "engine-123"
	runID := "run-123"

	engineServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v1/runs/"+engineRunID+"/export", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"input": {
				"scenario_yaml": "hosts:\n  - id: host-1\n    cores: 4\n"
			}
		}`))
	}))
	defer engineServer.Close()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	storage := &fakeObjectStorage{objects: make(map[string][]byte)}

	// Expect ensure simulation_runs row, then upsert into simulation_summaries
	mock.ExpectExec(`INSERT INTO simulation_runs`).
		WithArgs(driver.Value(runID)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO simulation_summaries`).
		WithArgs(driver.Value(runID), driver.Value(engineRunID), driver.Value("simulation/"+runID+"/best_scenario.yaml"), driver.Value("hosts:\n  - id: host-1\n    cores: 4\n")).
		WillReturnResult(sqlmock.NewResult(1, 1))

	h := &Handler{
		engineClient: NewSimulationEngineClient(engineServer.URL),
		db:           db,
		s3Client:     storage,
	}

	run := &domain.SimulationRun{
		RunID:       runID,
		EngineRunID: engineRunID,
	}

	h.uploadBestScenarioToS3(context.Background(), run, "completed", "")

	// Ensure object was stored in fake S3
	data, ok := storage.objects["simulation/"+runID+"/best_scenario.yaml"]
	require.True(t, ok)
	assert.Contains(t, string(data), "hosts:")

	require.NoError(t, mock.ExpectationsWereMet())
}

// TestUploadBestScenarioToS3_UsesBestRunIDWhenSet verifies that when bestRunID is set,
// the export is fetched for bestRunID rather than run.EngineRunID (batch optimization path).
func TestUploadBestScenarioToS3_UsesBestRunIDWhenSet(t *testing.T) {
	gin.SetMode(gin.TestMode)

	runID := "run-123"
	engineRunID := "engine-123"
	bestRunID := "best-789"

	var exportPath string
	engineServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		exportPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"input":{"scenario_yaml":"hosts:\n  - id: h1\n    cores: 2\n"}}`))
	}))
	defer engineServer.Close()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`INSERT INTO simulation_runs`).
		WithArgs(driver.Value(runID)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO simulation_summaries`).
		WithArgs(driver.Value(runID), driver.Value(engineRunID), driver.Value("simulation/"+runID+"/best_scenario.yaml"), driver.Value("hosts:\n  - id: h1\n    cores: 2\n")).
		WillReturnResult(sqlmock.NewResult(1, 1))

	h := &Handler{
		engineClient: NewSimulationEngineClient(engineServer.URL),
		db:           db,
		s3Client:     &fakeObjectStorage{objects: make(map[string][]byte)},
	}
	run := &domain.SimulationRun{
		RunID:       runID,
		EngineRunID: engineRunID,
	}

	h.uploadBestScenarioToS3(context.Background(), run, "completed", bestRunID)

	assert.Equal(t, "/v1/runs/"+bestRunID+"/export", exportPath, "export should be called with best_run_id, not engine_run_id")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestUniqueCandidateIDs verifies deduplication and best-first order.
func TestUniqueCandidateIDs(t *testing.T) {
	ids := uniqueCandidateIDs("best-1", []string{"best-1", "cand-2", "cand-2"})
	assert.Equal(t, []string{"best-1", "cand-2"}, ids)

	ids = uniqueCandidateIDs("", []string{"a", "b"})
	assert.Equal(t, []string{"a", "b"}, ids)

	ids = uniqueCandidateIDs("only", nil)
	assert.Equal(t, []string{"only"}, ids)
}

