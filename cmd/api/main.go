package main

import (
	"context"
	"log"
	"os"

	"github.com/GoSim-25-26J-441/go-sim-backend/config"
	cronjob "github.com/GoSim-25-26J-441/go-sim-backend/internal/analysis_suggestions/cron"
	httpapi "github.com/GoSim-25-26J-441/go-sim-backend/internal/api/http"
	as "github.com/GoSim-25-26J-441/go-sim-backend/internal/api/http/analysis_suggestions"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

const serviceName = "go-sim-backend"

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Set Gin mode based on environment
	if cfg.App.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.Default()

	// Configure CORS middleware
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:3000", "http://localhost:8080"}, // Your Next.js frontend URLs
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "Accept"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * 3600, // 12 hours
	}))

	// Handle preflight requests
	router.OPTIONS("/*path", func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, Accept")
		c.Status(204)
	})

	healthHandler := httpapi.NewHealthHandler(serviceName, cfg.App.Version)
	healthHandler.RegisterRoutes(router)

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatalf("DATABASE_URL must be set in environment")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("failed to create pgxpool: %v", err)
	}
	defer pool.Close()

	// Start cron scheduler
	scheduler := cronjob.NewScheduler()
	scheduler.Start()

	api := router.Group("/api")
	{
		fetchHandler := as.NewFetchHandler()
		fetchHandler.RegisterRoutes(api)

		importHandler := as.NewImportHandler()
		importHandler.RegisterRoutes(api)

		suggestHandler := as.NewSuggestHandler("internal/analysis_suggestions/rules/rules.json", pool)
		suggestHandler.RegisterRoutes(api)

		costHandler := as.NewCostHandler(pool)
		costHandler.RegisterRoutes(api)

		reqHandler := as.NewRequestHandler(pool)
		reqHandler.RegisterRoutes(api)
	}

	log.Printf("Starting %s v%s in %s mode", serviceName, cfg.App.Version, cfg.App.Environment)
	log.Printf("Server starting on port %s", cfg.Server.Port)
	log.Printf("CORS enabled for origins: http://localhost:3000, http://localhost:8080")
	log.Printf("Health endpoint available at: http://localhost:%s/health", cfg.Server.Port)
	log.Printf("Fetch endpoint available at: http://localhost:%s/api/fetch-prices", cfg.Server.Port)
	log.Printf("Import endpoint available at: http://localhost:%s/api/import-prices", cfg.Server.Port)
	log.Printf("Suggest endpoint available at: http://localhost:%s/api/suggest", cfg.Server.Port)
	log.Printf("Cost endpoint available at:    http://localhost:%s/api/cost/:id", cfg.Server.Port)
	log.Printf("Requests endpoint available at: http://localhost:%s/api/requests/user/:user_id", cfg.Server.Port)

	if err := router.Run(":" + cfg.Server.Port); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}
