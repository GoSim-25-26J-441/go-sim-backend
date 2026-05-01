package repository

import (
	"context"
	"database/sql"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/analysis_suggestions/hostconfig"
)

// LoadLatestScenarioHostConfig returns validated host sizing for the given user and project.
//
// Precedence:
//  1. Latest row with run_id IS NULL (design-only submissions), newest created_at — avoids run-scoped
//     rows that may carry stale copied design relative to a newer POST /design save.
//  2. If that row is missing or does not yield a valid host config, latest row regardless of run_id.
//
// Errors: non-nil err means a database/query failure on either SELECT; callers should not treat that
// as “no config” — surface or log it. ErrNoRows and invalid JSON/host fields yield err == nil with ok == false.
func LoadLatestScenarioHostConfig(ctx context.Context, db *sql.DB, userID, projectPublicID string) (hostconfig.ScenarioHostConfig, bool, error) {
	if db == nil || userID == "" || projectPublicID == "" {
		return hostconfig.ScenarioHostConfig{}, false, nil
	}

	const qDesignOnly = `
SELECT request
FROM request_responses
WHERE user_id = $1 AND COALESCE(project_id, '') = $2 AND run_id IS NULL
ORDER BY created_at DESC
LIMIT 1`

	const qLatestAny = `
SELECT request
FROM request_responses
WHERE user_id = $1 AND COALESCE(project_id, '') = $2
ORDER BY created_at DESC
LIMIT 1`

	var raw []byte
	err := db.QueryRowContext(ctx, qDesignOnly, userID, projectPublicID).Scan(&raw)
	if err != nil {
		if err != sql.ErrNoRows {
			return hostconfig.ScenarioHostConfig{}, false, err
		}
	} else {
		if cfg, ok := hostconfig.ParseScenarioHostConfig(raw); ok {
			return cfg, true, nil
		}
	}

	err = db.QueryRowContext(ctx, qLatestAny, userID, projectPublicID).Scan(&raw)
	if err != nil {
		if err == sql.ErrNoRows {
			return hostconfig.ScenarioHostConfig{}, false, nil
		}
		return hostconfig.ScenarioHostConfig{}, false, err
	}
	cfg, ok := hostconfig.ParseScenarioHostConfig(raw)
	return cfg, ok, nil
}
