package storage

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func SaveSuggestionRun(ctx context.Context, pool *pgxpool.Pool, design interface{}, candidates interface{}, allScores interface{}, best interface{}) (string, error) {
	if pool == nil {
		return "", fmt.Errorf("pgx pool is nil")
	}

	designB, err := json.Marshal(design)
	if err != nil {
		return "", fmt.Errorf("marshal design: %w", err)
	}
	candsB, err := json.Marshal(candidates)
	if err != nil {
		return "", fmt.Errorf("marshal candidates: %w", err)
	}
	allScoresB, err := json.Marshal(allScores)
	if err != nil {
		return "", fmt.Errorf("marshal allScores: %w", err)
	}
	bestB, err := json.Marshal(best)
	if err != nil {
		return "", fmt.Errorf("marshal best: %w", err)
	}

	var runID string
	sql := `
INSERT INTO suggestion_runs (design, candidates, all_scores, best_candidate)
VALUES ($1::jsonb, $2::jsonb, $3::jsonb, $4::jsonb)
RETURNING id;
`
	err = pool.QueryRow(ctx, sql, designB, candsB, allScoresB, bestB).Scan(&runID)
	if err != nil {
		return "", fmt.Errorf("insert suggestion run: %w", err)
	}
	return runID, nil
}

func InsertRunCandidates(ctx context.Context, pool *pgxpool.Pool, runID string, candidates []map[string]interface{}) error {
	if pool == nil {
		return fmt.Errorf("pgx pool is nil")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	sql := `
INSERT INTO suggestion_candidates (run_id, candidate_id, spec, metrics, sim_workload, source)
VALUES ($1, $2, $3::jsonb, $4::jsonb, $5::jsonb, $6)
`
	for _, c := range candidates {
		cid, _ := c["id"].(string)
		spec := c["spec"]
		metrics := c["metrics"]
		simWorkload := c["sim_workload"]
		source, _ := c["source"].(string)

		specB, _ := json.Marshal(spec)
		metricsB, _ := json.Marshal(metrics)
		simWorkloadB, _ := json.Marshal(simWorkload)

		if _, err := tx.Exec(ctx, sql, runID, cid, specB, metricsB, simWorkloadB, source); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}
