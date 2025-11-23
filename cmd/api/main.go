package main

import (
	"log"

	"github.com/gin-gonic/gin"

	"github.com/GoSim-25-26J-441/go-sim-backend/config"
	apihttp "github.com/GoSim-25-26J-441/go-sim-backend/internal/api/http"
	amgapd "github.com/GoSim-25-26J-441/go-sim-backend/internal/api/http/amg_apd"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	r := gin.Default()

	// Health endpoints
	healthHandler := apihttp.NewHealthHandler("amg-apd-service", cfg.App.Version)
	healthHandler.RegisterRoutes(r)

	// AMG & APD HTTP API
	amgapd.Register(r)

	log.Printf("listening on :%s", cfg.Server.Port)
	if err := r.Run(":" + cfg.Server.Port); err != nil {
		log.Fatalf("server exited with error: %v", err)
	}
}
