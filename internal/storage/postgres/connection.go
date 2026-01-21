package postgres

import (
	"database/sql"
	"fmt"

	"github.com/GoSim-25-26J-441/go-sim-backend/config"
	_ "github.com/lib/pq"
)

func NewConnection(cfg *config.DatabaseConfig) (*sql.DB, error) {
	dsn := DSN(cfg)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	return db, nil
}
