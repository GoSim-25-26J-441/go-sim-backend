package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

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

type AuthConfig struct {
	// LinkByEmail enables resolving an existing DB user by email when the Firebase UID from the
	// token is not found (useful for Firebase Auth Emulator/dev resets). When enabled, the app
	// will use the firebase_uid from the DB user row matching the token email.
	LinkByEmail bool
	// RequireEmailVerifiedForLinkByEmail blocks email-based linking unless the token has
	// email_verified=true. Strongly recommended for production.
	RequireEmailVerifiedForLinkByEmail bool
}

type UpstreamsConfig struct {
	LLMSvcURL           string
	LLMAPIKey           string
	SimulationEngineURL string
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

type SimulationCallbacksConfig struct {
	// CallbackURL is the full URL that the simulation engine should call when a run changes terminal state.
	// Example: http://localhost:8000/api/v1/simulation-engine/runs/callback
	CallbackURL string
	// CallbackSecret is a shared secret to authenticate callbacks from the simulation engine.
	// The engine should set header: X-Simulation-Callback-Secret: <secret>
	CallbackSecret string
}

// S3Config holds configuration for S3-compatible object storage (AWS S3, MinIO, etc.).
// Module-specific paths (e.g. projects/, exports/) can be configured via env in each module.
type S3Config struct {
	// Bucket is the S3 bucket name
	Bucket string
	// Region is the AWS region (e.g. us-east-1). Used for AWS S3; MinIO may ignore.
	Region string
	// Endpoint is optional. When set, uses a custom endpoint (e.g. MinIO: http://localhost:9000).
	// Leave empty for standard AWS S3.
	Endpoint string
	// ForcePathStyle when true uses path-style URLs (required for MinIO).
	ForcePathStyle bool
	// AccessKeyID and SecretAccessKey are optional. When both set, use static credentials.
	// Otherwise the SDK uses the default credential chain (env vars, ~/.aws/credentials, IAM roles).
	AccessKeyID     string
	SecretAccessKey string
}

type Config struct {
	Server    ServerConfig
	Database  DatabaseConfig
	App       AppConfig
	Auth      AuthConfig
	Upstreams UpstreamsConfig
	LLM       LLMConfig
	RAG       RAGConfig
	Firebase  FirebaseConfig
	Redis              RedisConfig
	SimulationCallbacks SimulationCallbacksConfig
	S3                  S3Config
}

func Load() (*Config, error) {
	// Load .env file if it exists (ignore error if file doesn't exist)
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found, using environment variables: %v", err)
	}

	cfg := &Config{
		Server: ServerConfig{
			Port: getEnv("PORT", "8000"),
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
		Auth: AuthConfig{
			LinkByEmail:                         getEnvAsBool("AUTH_LINK_BY_EMAIL", false),
			RequireEmailVerifiedForLinkByEmail:  getEnvAsBool("AUTH_LINK_REQUIRE_EMAIL_VERIFIED", true),
		},
		Upstreams: UpstreamsConfig{
			LLMSvcURL:           getEnv("LLM_SVC_URL", "http://localhost:8081"),
			LLMAPIKey:           getEnv("LLM_API_KEY", ""),
			SimulationEngineURL: getEnv("SIMULATION_ENGINE_URL", "http://localhost:8082"),
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
		SimulationCallbacks: SimulationCallbacksConfig{
			CallbackURL:    getEnv("SIMULATION_CALLBACK_URL", ""),
			CallbackSecret: getEnv("SIMULATION_CALLBACK_SECRET", ""),
		},
		S3: S3Config{
			Bucket:          getEnv("S3_BUCKET", ""),
			Region:          getEnv("AWS_REGION", getEnv("AWS_DEFAULT_REGION", "us-east-1")),
			Endpoint:        getEnv("S3_ENDPOINT", ""),
			ForcePathStyle:  getEnvAsBool("S3_FORCE_PATH_STYLE", false),
			AccessKeyID:     getEnv("AWS_ACCESS_KEY_ID", getEnv("S3_ACCESS_KEY_ID", "")),
			SecretAccessKey: getEnv("AWS_SECRET_ACCESS_KEY", getEnv("S3_SECRET_ACCESS_KEY", "")),
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

func getEnvAsBool(key string, defaultValue bool) bool {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	switch strings.ToLower(strings.TrimSpace(valueStr)) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		log.Printf("Warning: Invalid boolean for %s, using default: %v", key, defaultValue)
		return defaultValue
	}
}
