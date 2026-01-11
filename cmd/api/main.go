package main

import (
	"log"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"github.com/GoSim-25-26J-441/go-sim-backend/config"
	httpapi "github.com/GoSim-25-26J-441/go-sim-backend/internal/api/http"
	amgapd "github.com/GoSim-25-26J-441/go-sim-backend/internal/api/http/amg_apd"
)

const serviceName = "amg-apd-service"

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	if cfg.App.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.Default()

	corsConfig := cors.DefaultConfig()
	corsConfig.AllowOrigins = []string{
		"http://localhost:3000",
		"http://localhost:5173",
		"http://localhost:8080",
	}
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"}
	corsConfig.AllowHeaders = []string{
		"Origin", "Content-Type", "Content-Length", "Accept-Encoding",
		"Authorization", "accept", "origin", "Cache-Control", "X-Requested-With",
		"X-API-Key", "X-User-Id",
	}
	corsConfig.AllowCredentials = true
	corsConfig.MaxAge = 12 * time.Hour
	router.Use(cors.New(corsConfig))

	healthHandler := httpapi.NewHealthHandler(serviceName, cfg.App.Version)
	healthHandler.RegisterRoutes(router)

	amgapd.Register(router)

	log.Printf("Starting %s v%s in %s mode", serviceName, cfg.App.Version, cfg.App.Environment)
	log.Printf("Server starting on port %s", cfg.Server.Port)
	log.Printf("Health endpoint available at: http://localhost:%s/health", cfg.Server.Port)

	if err := router.Run(":" + cfg.Server.Port); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}
