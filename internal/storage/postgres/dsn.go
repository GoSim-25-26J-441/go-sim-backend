package postgres

import (
	"fmt"

	"github.com/GoSim-25-26J-441/go-sim-backend/config"
)

func DSN(cfg *config.DatabaseConfig) string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Name,
	)
}
