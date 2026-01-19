package main

import (
	"context"
	"log"
	"os"

	"github.com/GoSim-25-26J-441/go-sim-backend/config"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/bootstrap"
	"github.com/joho/godotenv"
)

const serviceName = "go-sim-backend"

func main() {

	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	bootstrap.SetGinMode(cfg.App.Environment)

	if err := bootstrap.LoadRAG(cfg.RAG.SnippetsDir); err != nil {
		log.Printf("RAG load: %v", err)
	}

	dbPool, err := bootstrap.OpenDB(context.Background(), bootstrap.DBOptions{
		DSN: os.Getenv("DB_DSN"),
	})
	if err != nil {
		log.Fatalf("DB init failed: %v", err)
	}
	defer dbPool.Close()

	router := bootstrap.BuildRouter(bootstrap.RouterDeps{
		ServiceName: serviceName,
		Version:     cfg.App.Version,
		UpstreamURL: cfg.Upstreams.LLMSvcURL,
		OllamaURL:   cfg.LLM.OllamaURL,
		DB:          dbPool,
	})

	log.Printf("Starting %s v%s in %s mode", serviceName, cfg.App.Version, cfg.App.Environment)
	log.Printf("Server starting on port %s", cfg.Server.Port)

	if err := router.Run(":" + cfg.Server.Port); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}
