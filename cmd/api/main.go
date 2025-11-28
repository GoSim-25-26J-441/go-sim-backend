package main

import (
	"log"

	"github.com/GoSim-25-26J-441/go-sim-backend/config"
	cronjob "github.com/GoSim-25-26J-441/go-sim-backend/internal/analysis_suggestions/cron"
	httpapi "github.com/GoSim-25-26J-441/go-sim-backend/internal/api/http"
	analysispapi "github.com/GoSim-25-26J-441/go-sim-backend/internal/api/http/analysis_suggestions"
	costcal "github.com/GoSim-25-26J-441/go-sim-backend/internal/api/http/analysis_suggestions"
	importer "github.com/GoSim-25-26J-441/go-sim-backend/internal/api/http/analysis_suggestions"
	"github.com/gin-gonic/gin"
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

	healthHandler := httpapi.NewHealthHandler(serviceName, cfg.App.Version)
	healthHandler.RegisterRoutes(router)

	// Start cron scheduler
	scheduler := cronjob.NewScheduler()
	scheduler.Start()

	api := router.Group("/api")
	{
		fetchHandler := analysispapi.NewFetchHandler()
		fetchHandler.RegisterRoutes(api)

		importHandler := importer.NewImportHandler()
		importHandler.RegisterRoutes(api)

		suggestHandler := analysispapi.NewSuggestHandler("internal/analysis_suggestions/rules/rules.json")
		suggestHandler.RegisterRoutes(api)

		costHandler := costcal.NewCostHandler()
		costHandler.RegisterRoutes(api)
	}

	log.Printf("Starting %s v%s in %s mode", serviceName, cfg.App.Version, cfg.App.Environment)
	log.Printf("Server starting on port %s", cfg.Server.Port)
	log.Printf("Health endpoint available at: http://localhost:%s/health", cfg.Server.Port)
	log.Printf("Fetch endpoint available at: http://localhost:%s/api/fetch-prices", cfg.Server.Port)
	log.Printf("Import endpoint available at: http://localhost:%s/api/import-prices", cfg.Server.Port)
	log.Printf("Suggest endpoint available at: http://localhost:%s/api/suggest", cfg.Server.Port)
	log.Printf("Cost endpoint available at:    http://localhost:%s/api/cost/:id", cfg.Server.Port)

	if err := router.Run(":" + cfg.Server.Port); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}
