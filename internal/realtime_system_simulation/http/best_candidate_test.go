package http

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/domain"
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


func TestHandler_GetBestCandidate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	runID := "run-123"
	s3Key := "simulation/run-123/best_scenario.yaml"

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
		objects: map[string][]byte{
			s3Key: []byte(yamlContent),
		},
	}

	h := &Handler{
		db:       db,
		s3Client: storage,
	}

	router := gin.New()
	router.GET("/runs/:id/best-candidate", h.GetBestCandidate)

	req := httptest.NewRequest(http.MethodGet, "/runs/"+runID+"/best-candidate", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		RunID        string `json:"run_id"`
		BestCandidate struct {
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
		} `json:"best_candidate"`
	}

	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, runID, resp.RunID)
	assert.Equal(t, s3Key, resp.BestCandidate.S3Path)
	require.Len(t, resp.BestCandidate.Hosts, 2)
	assert.Equal(t, "host-1", resp.BestCandidate.Hosts[0].HostID)
	assert.Equal(t, 4, resp.BestCandidate.Hosts[0].CPUCores)
	assert.Equal(t, 16.0, resp.BestCandidate.Hosts[0].MemoryGB)

	require.Len(t, resp.BestCandidate.Services, 1)
	assert.Equal(t, "svc1", resp.BestCandidate.Services[0].ServiceID)
	assert.Equal(t, 3, resp.BestCandidate.Services[0].Replicas)
	assert.Equal(t, 1.0, resp.BestCandidate.Services[0].CPUCores)
	assert.Equal(t, 512.0, resp.BestCandidate.Services[0].MemoryMB)

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

	// Expect upsert into simulation_summaries
	mock.ExpectExec(`INSERT INTO simulation_summaries`).
		WithArgs(driver.Value(runID), driver.Value(engineRunID), driver.Value("simulation/"+runID+"/best_scenario.yaml")).
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

	h.uploadBestScenarioToS3(context.Background(), run, "completed")

	// Ensure object was stored in fake S3
	data, ok := storage.objects["simulation/"+runID+"/best_scenario.yaml"]
	require.True(t, ok)
	assert.Contains(t, string(data), "hosts:")

	require.NoError(t, mock.ExpectationsWereMet())
}


