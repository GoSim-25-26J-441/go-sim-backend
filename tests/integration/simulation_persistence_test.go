package integration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/domain"
	simrepo "github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/repository"
	simservice "github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/service"
	"github.com/alicebob/miniredis/v2"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestPostgres creates a test PostgreSQL connection
// Skips test if TEST_DB_DSN is not set
// You can set TEST_DB_DSN directly, or use individual env vars:
//
//	TEST_DB_HOST, TEST_DB_PORT, TEST_DB_USER, TEST_DB_PASSWORD, TEST_DB_NAME
func setupTestPostgres(t *testing.T) *sql.DB {
	dsn := os.Getenv("TEST_DB_DSN")

	// If TEST_DB_DSN is not set, try to construct it from individual env vars
	if dsn == "" {
		host := os.Getenv("TEST_DB_HOST")
		port := os.Getenv("TEST_DB_PORT")
		user := os.Getenv("TEST_DB_USER")
		password := os.Getenv("TEST_DB_PASSWORD")
		dbname := os.Getenv("TEST_DB_NAME")

		// If all individual vars are set, construct DSN
		if host != "" && port != "" && user != "" && dbname != "" {
			dsn = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
				host, port, user, password, dbname)
		} else {
			// Fall back to regular DB config if TEST_DB_* vars not set
			host = os.Getenv("DB_HOST")
			port = os.Getenv("DB_PORT")
			user = os.Getenv("DB_USER")
			password = os.Getenv("DB_PASSWORD")
			dbname = os.Getenv("DB_NAME")

			if host != "" && port != "" && user != "" && dbname != "" {
				dsn = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
					host, port, user, password, dbname)
			} else {
				t.Skip("TEST_DB_DSN or DB_* environment variables not set, skipping PostgreSQL integration test")
			}
		}
	}

	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)

	// Test connection
	err = db.Ping()
	require.NoError(t, err)

	// Run migrations (create tables if they don't exist)
	// In a real scenario, you'd run migrations before tests
	// For now, we assume tables exist or will be created by the test

	return db
}

// insertRunIntoPostgres inserts a run_id into the simulation_runs table
// This is required for foreign key constraints when persisting summaries/metrics
func insertRunIntoPostgres(ctx context.Context, db *sql.DB, runID string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO simulation_runs (run_id, created_at)
		VALUES ($1, NOW())
		ON CONFLICT (run_id) DO NOTHING
	`, runID)
	return err
}

// setupTestServiceWithPersistence creates a service with persistence support
func setupTestServiceWithPersistence(t *testing.T) (*simservice.SimulationService, *redis.Client, *miniredis.Miniredis, *sql.DB) {
	// Setup Redis
	redisClient, mr := setupTestRedis(t)

	// Setup PostgreSQL
	db := setupTestPostgres(t)

	// Create repositories
	runRepo := simrepo.NewRunRepository(redisClient)
	summaryRepo := simrepo.NewSummaryRepository(db)
	metricsRepo := simrepo.NewMetricsTimeseriesRepository(db)

	// Create mock engine client adapter
	// In real tests, you might want to use a real engine client or a more sophisticated mock
	engineClient := &mockEngineClient{}

	// Create service with persistence
	service := simservice.NewSimulationServiceWithPersistence(
		runRepo,
		summaryRepo,
		metricsRepo,
		engineClient,
	)

	return service, redisClient, mr, db
}

// mockEngineClient implements the EngineClient interface for testing
type mockEngineClient struct{}

func (m *mockEngineClient) GetRunSummary(engineRunID string) (*simservice.RunSummaryResponse, error) {
	return &simservice.RunSummaryResponse{
		Summary: struct {
			RunID           string                 `json:"run_id"`
			TotalRequests   int64                  `json:"total_requests,omitempty"`
			TotalErrors     int64                  `json:"total_errors,omitempty"`
			TotalDurationMs int64                  `json:"total_duration_ms,omitempty"`
			Metrics         map[string]interface{} `json:"metrics,omitempty"`
			SummaryData     map[string]interface{} `json:"summary_data,omitempty"`
			CreatedAtUnixMs int64                  `json:"created_at_unix_ms,omitempty"`
			StartedAtUnixMs int64                  `json:"started_at_unix_ms,omitempty"`
			EndedAtUnixMs   int64                  `json:"ended_at_unix_ms,omitempty"`
		}{
			RunID:           engineRunID,
			TotalRequests:   1000,
			TotalErrors:     10,
			TotalDurationMs: 120000,
			Metrics: map[string]interface{}{
				"request_latency": map[string]interface{}{
					"avg": 125.5,
					"p95": 250.0,
				},
			},
		},
	}, nil
}

func (m *mockEngineClient) GetRunMetrics(engineRunID string) ([]simservice.MetricDataPointResponse, error) {
	now := time.Now()
	return []simservice.MetricDataPointResponse{
		{
			TimestampMs: now.UnixMilli(),
			MetricType:  "request_latency_ms",
			MetricValue: 125.5,
		},
		{
			TimestampMs: now.Add(time.Second).UnixMilli(),
			MetricType:  "cpu_utilization",
			MetricValue: 65.2,
		},
	}, nil
}

func TestSimulationService_StoreRunSummaryAndMetrics(t *testing.T) {
	service, redisClient, mr, db := setupTestServiceWithPersistence(t)
	defer mr.Close()
	defer redisClient.Close()
	defer db.Close()

	ctx := context.Background()

	t.Run("stores summary and metrics when run completes", func(t *testing.T) {
		// Create a run
		req := &domain.CreateRunRequest{
			UserID: "user123",
		}
		run, err := service.CreateRun(req)
		require.NoError(t, err)

		// Insert run into PostgreSQL for foreign key constraint
		err = insertRunIntoPostgres(ctx, db, run.RunID)
		require.NoError(t, err)

		// Set engine run ID
		engineRunID := "engine-123"
		updateReq := &domain.UpdateRunRequest{
			EngineRunID: &engineRunID,
		}
		run, err = service.UpdateRun(run.RunID, updateReq)
		require.NoError(t, err)

		// Store summary and metrics (simulating completion)
		err = service.StoreRunSummaryAndMetrics(ctx, run.RunID)
		require.NoError(t, err)

		// Verify summary was stored
		summary, err := service.GetStoredSummary(run.RunID)
		require.NoError(t, err)
		assert.Equal(t, run.RunID, summary.RunID)
		assert.Equal(t, int64(1000), summary.TotalRequests)
		assert.Equal(t, int64(10), summary.TotalErrors)

		// Verify metrics were stored
		metrics, err := service.GetStoredMetrics(ctx, run.RunID, "")
		require.NoError(t, err)
		assert.Greater(t, len(metrics), 0)
	})

	t.Run("returns error when run has no engine run ID", func(t *testing.T) {
		req := &domain.CreateRunRequest{
			UserID: "user123",
		}
		run, err := service.CreateRun(req)
		require.NoError(t, err)

		// Try to store without engine run ID
		err = service.StoreRunSummaryAndMetrics(ctx, run.RunID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no engine run ID")
	})
}

func TestSimulationService_GetStoredSummary(t *testing.T) {
	service, redisClient, mr, db := setupTestServiceWithPersistence(t)
	defer mr.Close()
	defer redisClient.Close()
	defer db.Close()

	t.Run("retrieves stored summary", func(t *testing.T) {
		// Create and store a summary
		req := &domain.CreateRunRequest{UserID: "user123"}
		run, err := service.CreateRun(req)
		require.NoError(t, err)

		// Insert run into PostgreSQL for foreign key constraint
		err = insertRunIntoPostgres(context.Background(), db, run.RunID)
		require.NoError(t, err)

		engineRunID := "engine-123"
		updateReq := &domain.UpdateRunRequest{EngineRunID: &engineRunID}
		_, err = service.UpdateRun(run.RunID, updateReq)
		require.NoError(t, err)

		// Store summary
		err = service.StoreRunSummaryAndMetrics(context.Background(), run.RunID)
		require.NoError(t, err)

		// Retrieve summary
		summary, err := service.GetStoredSummary(run.RunID)
		require.NoError(t, err)
		assert.Equal(t, run.RunID, summary.RunID)
		assert.Equal(t, engineRunID, summary.EngineRunID)
	})

	t.Run("returns error for non-existent summary", func(t *testing.T) {
		_, err := service.GetStoredSummary("non-existent-run-id")
		assert.Error(t, err)
		assert.Equal(t, domain.ErrRunNotFound, err)
	})
}

func TestSimulationService_GetStoredMetrics(t *testing.T) {
	service, redisClient, mr, db := setupTestServiceWithPersistence(t)
	defer mr.Close()
	defer redisClient.Close()
	defer db.Close()

	ctx := context.Background()

	t.Run("retrieves stored metrics", func(t *testing.T) {
		// Create run and store metrics
		req := &domain.CreateRunRequest{UserID: "user123"}
		run, err := service.CreateRun(req)
		require.NoError(t, err)

		// Insert run into PostgreSQL for foreign key constraint
		err = insertRunIntoPostgres(ctx, db, run.RunID)
		require.NoError(t, err)

		engineRunID := "engine-123"
		updateReq := &domain.UpdateRunRequest{EngineRunID: &engineRunID}
		_, err = service.UpdateRun(run.RunID, updateReq)
		require.NoError(t, err)

		// Store summary and metrics
		err = service.StoreRunSummaryAndMetrics(ctx, run.RunID)
		require.NoError(t, err)

		// Retrieve all metrics
		metrics, err := service.GetStoredMetrics(ctx, run.RunID, "")
		require.NoError(t, err)
		assert.Greater(t, len(metrics), 0)

		// Retrieve filtered metrics
		latencyMetrics, err := service.GetStoredMetrics(ctx, run.RunID, "request_latency_ms")
		require.NoError(t, err)
		if len(latencyMetrics) > 0 {
			assert.Equal(t, "request_latency_ms", latencyMetrics[0].MetricType)
		}
	})
}
