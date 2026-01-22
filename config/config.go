package config

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type ServerConfig struct {
	Port string
}

type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
}

type AppConfig struct {
	Environment string
	LogLevel    string
	Version     string
}

type UpstreamsConfig struct {
	LLMSvcURL           string
	SimulationEngineURL string
}

type SimulationCallbacksConfig struct {
	CallbackURL    string // URL for simulation engine to call back to
	CallbackSecret string // Secret for authenticating callbacks (optional)
}

type LLMConfig struct {
	OllamaURL string
}

type RAGConfig struct {
	SnippetsDir string
}

type FirebaseConfig struct {
	CredentialsPath string // Path to Firebase service account JSON
	ProjectID       string // Firebase project ID (optional, can be in credentials)
}

type RedisConfig struct {
	Host     string
	Port     int
	Password string
	DB       int // Database number (0-15)
}

type Config struct {
	Server            ServerConfig
	Database         DatabaseConfig
	App              AppConfig
	Upstreams        UpstreamsConfig
	SimulationCallbacks SimulationCallbacksConfig
	LLM              LLMConfig
	RAG              RAGConfig
	Firebase         FirebaseConfig
	Redis            RedisConfig
}

func Load() (*Config, error) {
	// Load .env file if it exists (ignore error if file doesn't exist)
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found, using environment variables: %v", err)
	}

	cfg := &Config{
		Server: ServerConfig{
			Port: getEnv("PORT", "8080"),
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnvAsInt("DB_PORT", 5432),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", ""),
			Name:     getEnv("DB_NAME", "gosim"),
		},
		App: AppConfig{
			Environment: getEnv("APP_ENV", "development"),
			LogLevel:    getEnv("LOG_LEVEL", "info"),
			Version:     getEnv("APP_VERSION", "1.0.0"),
		},
		Upstreams: UpstreamsConfig{
			LLMSvcURL:           getEnv("LLM_SVC_URL", "http://localhost:8081"),
			SimulationEngineURL: getEnv("SIMULATION_ENGINE_URL", "http://localhost:8082"),
		},
		SimulationCallbacks: SimulationCallbacksConfig{
			CallbackURL:    getEnv("SIMULATION_CALLBACK_URL", ""),
			CallbackSecret: getEnv("SIMULATION_CALLBACK_SECRET", ""),
		},
		LLM: LLMConfig{
			OllamaURL: getEnv("OLLAMA_URL", "http://localhost:11434"),
		},
		RAG: RAGConfig{
			SnippetsDir: getEnv("RAG_SNIPPETS_DIR", "internal/design_input_processing/rag/snippets"),
		},
		Firebase: FirebaseConfig{
			CredentialsPath: getEnv("FIREBASE_CREDENTIALS_PATH", ""),
			ProjectID:       getEnv("FIREBASE_PROJECT_ID", ""),
		},
		Redis: RedisConfig{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnvAsInt("REDIS_PORT", 6379),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvAsInt("REDIS_DB", 0),
		},
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if c.Server.Port == "" {
		return fmt.Errorf("PORT is required")
	}

	if c.Database.Host == "" {
		return fmt.Errorf("DB_HOST is required")
	}

	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}

	value, err := strconv.Atoi(valueStr)
	if err != nil {
		log.Printf("Warning: Invalid integer for %s, using default: %d", key, defaultValue)
		return defaultValue
	}

	return value
}
