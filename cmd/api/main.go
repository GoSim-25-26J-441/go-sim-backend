package main

import (
	"log"

	"firebase.google.com/go/v4/auth"
	"github.com/GoSim-25-26J-441/go-sim-backend/config"
	httpapi "github.com/GoSim-25-26J-441/go-sim-backend/internal/api/http"
	authpkg "github.com/GoSim-25-26J-441/go-sim-backend/internal/auth"
	authhttp "github.com/GoSim-25-26J-441/go-sim-backend/internal/auth/http"
	authmiddleware "github.com/GoSim-25-26J-441/go-sim-backend/internal/auth/middleware"
	authrepo "github.com/GoSim-25-26J-441/go-sim-backend/internal/auth/repository"
	authservice "github.com/GoSim-25-26J-441/go-sim-backend/internal/auth/service"
	diphttp "github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/http"
	dipmiddleware "github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/middleware"
	diprag "github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/rag"
	simhttp "github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/http"
	simrepo "github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/repository"
	simservice "github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/service"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/storage/postgres"
	redisstorage "github.com/GoSim-25-26J-441/go-sim-backend/internal/storage/redis"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

const serviceName = "go-sim-backend"

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	if cfg.App.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Load RAG snippets before starting server
	if err := diprag.Load(cfg.RAG.SnippetsDir); err != nil {
		log.Printf("RAG load: %v", err)
	}

	// Initialize database connection
	db, err := postgres.NewConnection(&cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()
	log.Printf("Database connection established")

	// Initialize Redis connection
	redisClient, err := redisstorage.NewConnection(&cfg.Redis)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer redisClient.Close()
	log.Printf("Redis connection established")

	// Initialize Firebase Auth (optional - only if credentials are provided)
	var authClient interface{}
	if cfg.Firebase.CredentialsPath != "" {
		fbAuth, err := authpkg.InitializeFirebase(&cfg.Firebase)
		if err != nil {
			log.Printf("Warning: Failed to initialize Firebase: %v (auth endpoints will be disabled)", err)
		} else {
			authClient = fbAuth
			log.Printf("Firebase Auth initialized")
		}
	} else {
		log.Printf("Firebase credentials not provided (auth endpoints will be disabled)")
	}

	router := gin.Default()

	// Configure CORS middleware
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowOrigins = []string{"http://localhost:3000", "http://localhost:5173", "http://localhost:8080"} // Common frontend dev ports
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"}
	corsConfig.AllowHeaders = []string{"Origin", "Content-Type", "Content-Length", "Accept-Encoding", "X-CSRF-Token", "Authorization", "accept", "origin", "Cache-Control", "X-Requested-With", "X-API-Key", "X-User-Id"}
	corsConfig.AllowCredentials = true
	corsConfig.MaxAge = 12 * 60 * 60 // 12 hours
	router.Use(cors.New(corsConfig))

	healthHandler := httpapi.NewHealthHandler(serviceName, cfg.App.Version)
	healthHandler.RegisterRoutes(router)

	api := router.Group("/api/v1")

	// Design Input Processing routes
	dip := api.Group("/design-input")
	dip.Use(dipmiddleware.APIKeyMiddleware())
	dip.Use(dipmiddleware.RequestIDMiddleware())
	dipHandler := diphttp.New(cfg.Upstreams.LLMSvcURL, cfg.LLM.OllamaURL)
	dipHandler.Register(dip)

	// Auth routes (only if Firebase is initialized)
	if authClient != nil {
		authGroup := api.Group("/auth")

		// Initialize auth module
		userRepo := authrepo.NewUserRepository(db)
		authService := authservice.NewAuthService(userRepo)
		authHandler := authhttp.New(authService)

		// Apply Firebase Auth middleware to auth routes
		authGroup.Use(authmiddleware.FirebaseAuthMiddleware(authClient.(*auth.Client)))
		authHandler.Register(authGroup)

		log.Printf("Auth endpoints registered at /api/v1/auth")
	}

	// Initialize simulation module (required for both user routes and callback routes)
	simRunRepo := simrepo.NewRunRepository(redisClient)
	simService := simservice.NewSimulationService(simRunRepo)
	simHandler := simhttp.New(
		simService,
		cfg.Upstreams.SimulationEngineURL,
		cfg.SimulationCallbacks.CallbackURL,
		cfg.SimulationCallbacks.CallbackSecret,
		redisClient,
	)

	// Simulation-engine callback routes (called by simulation engine, NOT by end-users)
	// These routes should NOT require Firebase auth - they're called by the simulator
	// Authentication is handled via callback secret (X-Simulation-Callback-Secret header)
	simEngineGroup := api.Group("/simulation-engine")
	simHandler.RegisterEngineCallbackRoutes(simEngineGroup)
	log.Printf("Simulation engine callback endpoints registered at /api/v1/simulation-engine/runs/callback (no Firebase auth required)")

	// Simulation routes (user-facing endpoints - require Firebase auth if Firebase is initialized)
	if authClient != nil {
		simGroup := api.Group("/simulation")

		// Apply Firebase Auth middleware to simulation routes (for user access)
		simGroup.Use(authmiddleware.FirebaseAuthMiddleware(authClient.(*auth.Client)))

		simHandler.Register(simGroup)

		log.Printf("Simulation user endpoints registered at /api/v1/simulation (Firebase auth required)")
	} else {
		log.Printf("Simulation user endpoints disabled (Firebase not initialized)")
	}

	log.Printf("Starting %s v%s in %s mode", serviceName, cfg.App.Version, cfg.App.Environment)
	log.Printf("Server starting on port %s", cfg.Server.Port)
	log.Printf("Health endpoint available at: http://localhost:%s/health", cfg.Server.Port)

	if err := router.Run(":" + cfg.Server.Port); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}
