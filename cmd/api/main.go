package main

import (
	"log"

	"github.com/GoSim-25-26J-441/go-sim-backend/config"
	httpapi "github.com/GoSim-25-26J-441/go-sim-backend/internal/api/http"
	diphttp "github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/http"
	diprag "github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/rag"
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

	router := gin.Default()

	healthHandler := httpapi.NewHealthHandler(serviceName, cfg.App.Version)
	healthHandler.RegisterRoutes(router)

	api := router.Group("/api/v1")

	dip := api.Group("/design-input")
	dipHandler := diphttp.New(cfg.Upstreams.LLMSvcURL, cfg.LLM.OllamaURL)
	dipHandler.Register(dip)

	log.Printf("Starting %s v%s in %s mode", serviceName, cfg.App.Version, cfg.App.Environment)
	log.Printf("Server starting on port %s", cfg.Server.Port)
	log.Printf("Health endpoint available at: http://localhost:%s/health", cfg.Server.Port)

	if err := router.Run(":" + cfg.Server.Port); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}
