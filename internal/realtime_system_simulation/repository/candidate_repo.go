package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// CandidateRecord represents a row in simulation_candidates.
type CandidateRecord struct {
	ID              string
	UserID          string
	ProjectPublicID sql.NullString
	RunID           string
	CandidateID     string
	Spec            map[string]any
	Metrics         map[string]any
	SimWorkload     map[string]any
	Source          string
}

// CandidateRepository handles Postgres operations for simulation candidates.
type CandidateRepository struct {
	db *sql.DB
}

// NewCandidateRepository creates a new CandidateRepository backed by the given DB.
func NewCandidateRepository(db *sql.DB) *CandidateRepository {
	return &CandidateRepository{db: db}
}

// CreateMany inserts a batch of candidate records for a run.
func (r *CandidateRepository) CreateMany(ctx context.Context, records []*CandidateRecord) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("candidate repository not initialized")
	}
	if len(records) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction for candidates insert: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	stmt, err := tx.PrepareContext(ctx, `
        INSERT INTO simulation_candidates
            (user_id, project_public_id, run_id, candidate_id, spec, metrics, sim_workload, source)
        VALUES
            ($1, $2, $3, $4, $5::jsonb, $6::jsonb, $7::jsonb, $8)
        ON CONFLICT (run_id, candidate_id) DO UPDATE
        SET spec = EXCLUDED.spec,
            metrics = EXCLUDED.metrics,
            sim_workload = EXCLUDED.sim_workload,
            source = EXCLUDED.source,
            updated_at = NOW()
    `)
	if err != nil {
		return fmt.Errorf("failed to prepare candidates insert statement: %w", err)
	}
	defer stmt.Close()

	for _, rec := range records {
		specJSON, err := json.Marshal(rec.Spec)
		if err != nil {
			return fmt.Errorf("failed to marshal spec JSON for candidate_id=%s: %w", rec.CandidateID, err)
		}
		metricsJSON, err := json.Marshal(rec.Metrics)
		if err != nil {
			return fmt.Errorf("failed to marshal metrics JSON for candidate_id=%s: %w", rec.CandidateID, err)
		}
		workloadJSON, err := json.Marshal(rec.SimWorkload)
		if err != nil {
			return fmt.Errorf("failed to marshal sim_workload JSON for candidate_id=%s: %w", rec.CandidateID, err)
		}

		if _, err := stmt.ExecContext(
			ctx,
			rec.UserID,
			rec.ProjectPublicID,
			rec.RunID,
			rec.CandidateID,
			string(specJSON),
			string(metricsJSON),
			string(workloadJSON),
			rec.Source,
		); err != nil {
			return fmt.Errorf("failed to insert candidate run_id=%s candidate_id=%s: %w", rec.RunID, rec.CandidateID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit candidates insert transaction: %w", err)
	}

	return nil
}

// ListByRunID returns all candidates associated with the given run ID.
func (r *CandidateRepository) ListByRunID(ctx context.Context, runID string) ([]*CandidateRecord, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("candidate repository not initialized")
	}

	rows, err := r.db.QueryContext(ctx, `
        SELECT id, user_id, project_public_id, run_id, candidate_id,
               spec, metrics, sim_workload, source
        FROM simulation_candidates
        WHERE run_id = $1
        ORDER BY candidate_id
    `, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to query simulation_candidates for run_id=%s: %w", runID, err)
	}
	defer rows.Close()

	var out []*CandidateRecord
	for rows.Next() {
		var rec CandidateRecord
		var specJSON, metricsJSON, workloadJSON []byte

		if err := rows.Scan(
			&rec.ID,
			&rec.UserID,
			&rec.ProjectPublicID,
			&rec.RunID,
			&rec.CandidateID,
			&specJSON,
			&metricsJSON,
			&workloadJSON,
			&rec.Source,
		); err != nil {
			return nil, fmt.Errorf("failed to scan simulation_candidates row: %w", err)
		}

		if err := json.Unmarshal(specJSON, &rec.Spec); err != nil {
			return nil, fmt.Errorf("failed to unmarshal spec JSON for candidate_id=%s: %w", rec.CandidateID, err)
		}
		if err := json.Unmarshal(metricsJSON, &rec.Metrics); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metrics JSON for candidate_id=%s: %w", rec.CandidateID, err)
		}
		if err := json.Unmarshal(workloadJSON, &rec.SimWorkload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal sim_workload JSON for candidate_id=%s: %w", rec.CandidateID, err)
		}

		out = append(out, &rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating simulation_candidates rows: %w", err)
	}

	return out, nil
}

