package http

import (
	"context"
	"database/sql"

	simrepo "github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/repository"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/service"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/storage/s3"
	"github.com/redis/go-redis/v9"
)

// ObjectStorage defines the minimal S3-like interface needed for best-candidate storage.
type ObjectStorage interface {
	PutObject(ctx context.Context, key string, data []byte) error
	GetObject(ctx context.Context, key string) ([]byte, error)
}

// Handler handles HTTP requests for simulation runs
type Handler struct {
	simService          *service.SimulationService
	simulationEngineURL string
	engineClient        *SimulationEngineClient
	callbackURL         string
	callbackSecret      string
	redisClient         *redis.Client
	db                  *sql.DB
	s3Client            ObjectStorage
	scenarioCacheRepo   *simrepo.ScenarioCacheRepository
}

// New creates a new Handler
func New(simService *service.SimulationService, simulationEngineURL string, callbackURL string, callbackSecret string, redisClient *redis.Client, db *sql.DB, s3Client *s3.Client) *Handler {
	return &Handler{
		simService:          simService,
		simulationEngineURL: simulationEngineURL,
		engineClient:        NewSimulationEngineClient(simulationEngineURL),
		callbackURL:         callbackURL,
		callbackSecret:      callbackSecret,
		redisClient:         redisClient,
		db:                  db,
		s3Client:            s3Client,
		scenarioCacheRepo:   simrepo.NewScenarioCacheRepository(db),
	}
}

// NewWithDeps allows tests to inject optional dependencies.
func NewWithDeps(simService *service.SimulationService, simulationEngineURL string, callbackURL string, callbackSecret string, redisClient *redis.Client, db *sql.DB, s3Client ObjectStorage, scenarioCacheRepo *simrepo.ScenarioCacheRepository) *Handler {
	if scenarioCacheRepo == nil {
		scenarioCacheRepo = simrepo.NewScenarioCacheRepository(db)
	}
	return &Handler{
		simService:          simService,
		simulationEngineURL: simulationEngineURL,
		engineClient:        NewSimulationEngineClient(simulationEngineURL),
		callbackURL:         callbackURL,
		callbackSecret:      callbackSecret,
		redisClient:         redisClient,
		db:                  db,
		s3Client:            s3Client,
		scenarioCacheRepo:   scenarioCacheRepo,
	}
}
