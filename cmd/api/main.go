package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	fbauth "firebase.google.com/go/v4/auth"
	"github.com/joho/godotenv"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/GoSim-25-26J-441/go-sim-backend/config"
	httpapi "github.com/GoSim-25-26J-441/go-sim-backend/internal/api/http"
	apiroutes "github.com/GoSim-25-26J-441/go-sim-backend/internal/api/http/routes"

	authpkg "github.com/GoSim-25-26J-441/go-sim-backend/internal/auth"
	dipllm "github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/llm"
	diprag "github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/rag"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/storage/postgres"
)

const serviceName = "go-sim-backend"

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	if cfg.App.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	if err := diprag.Load(cfg.RAG.SnippetsDir); err != nil {
		log.Printf("RAG load: %v", err)
	}

	// 1) Auth DB (sql.DB) - only if your auth module still uses database/sql
	var authSQL *sql.DB
	authSQL, err = postgres.NewConnection(&cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect auth sql db: %v", err)
	}
	defer authSQL.Close()

	// 2) App DB (pgxpool) - projects/diagrams/chats/users
	dsn := pgxDSN(&cfg.Database)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dbPool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("pgx connect: %v", err)
	}
	defer dbPool.Close()

	// Firebase (optional)
	var fbClient *fbauth.Client
	if cfg.Firebase.CredentialsPath != "" {
		c, err := authpkg.InitializeFirebase(&cfg.Firebase)
		if err != nil {
			log.Printf("Warning: Firebase init failed: %v", err)
		} else {
			fbClient = c
		}
	}

	// Router
	r := gin.Default()
	r.Use(cors.New(buildCORSConfig()))

	healthHandler := httpapi.NewHealthHandler(serviceName, cfg.App.Version, dbPool)

	healthHandler.RegisterRoutes(r)
	healthHandler.RegisterRoutes(r.Group("/api/v1"))

	apiroutes.RegisterV1(r, apiroutes.V1Deps{
		DBPool:   dbPool,
		AuthSQL:  authSQL,
		Firebase: fbClient,
		UIGP:     dipllm.NewUIGP(),
	})

	log.Printf("Server starting on port %s", cfg.Server.Port)
	if err := r.Run(":" + cfg.Server.Port); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}

func pgxDSN(db *config.DatabaseConfig) string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		db.Host, db.Port, db.User, db.Password, db.Name,
	)
}

func buildCORSConfig() cors.Config {
	c := cors.DefaultConfig()
	c.AllowOrigins = []string{"http://localhost:3000", "http://localhost:5173", "http://localhost:8080"}
	c.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"}
	c.AllowHeaders = []string{
		"Origin", "Content-Type", "Content-Length", "Accept-Encoding",
		"Authorization", "X-API-Key", "X-User-Id", "X-CSRF-Token",
		"accept", "origin", "Cache-Control", "X-Requested-With",
	}
	c.AllowCredentials = true
	c.MaxAge = 12 * time.Hour
	return c
}
