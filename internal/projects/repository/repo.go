package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/projects/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/projects/utils"
)

// ProjectRepository provides persistence operations for projects
type ProjectRepository struct {
	db *sql.DB
}

// NewProjectRepository creates a new project repository
func NewProjectRepository(db *sql.DB) *ProjectRepository {
	return &ProjectRepository{db: db}
}

// New is an alias for NewProjectRepository for backward compatibility
func New(db *sql.DB) *ProjectRepository {
	return NewProjectRepository(db)
}

// Create inserts a new project for the given user.
func (r *ProjectRepository) Create(ctx context.Context, userFirebaseUID, name string, temporary bool) (*domain.Project, error) {
	if name == "" {
		return nil, fmt.Errorf("name required")
	}
	if userFirebaseUID == "" {
		return nil, fmt.Errorf("user firebase uid required")
	}

	for i := 0; i < 5; i++ {
		publicID, err := utils.NewTextID("archfind")
		if err != nil {
			return nil, err
		}

		const q = `
INSERT INTO projects (public_id, user_firebase_uid, name, is_temporary)
VALUES ($1, $2, $3, $4)
RETURNING public_id, name, is_temporary, created_at, updated_at;
`
		var p domain.Project
		err = r.db.QueryRowContext(ctx, q, publicID, userFirebaseUID, name, temporary).
			Scan(&p.PublicID, &p.Name, &p.Temporary, &p.CreatedAt, &p.UpdatedAt)

		if err == nil {
			return &p, nil
		}

		// unique violation on public_id → retry
		var pgErr *pq.Error
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			continue
		}
		return nil, err
	}

	return nil, fmt.Errorf("failed to generate unique project id")
}

// List returns all non-deleted projects for the given user.
func (r *ProjectRepository) List(ctx context.Context, userFirebaseUID string) ([]domain.Project, error) {
	const q = `
SELECT public_id, name, is_temporary, created_at, updated_at
FROM projects
WHERE user_firebase_uid = $1 AND deleted_at IS NULL
ORDER BY created_at DESC;
`
	rows, err := r.db.QueryContext(ctx, q, userFirebaseUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.Project, 0, 16)
	for rows.Next() {
		var p domain.Project
		if err := rows.Scan(&p.PublicID, &p.Name, &p.Temporary, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Rename updates the project's name.
func (r *ProjectRepository) Rename(ctx context.Context, userFirebaseUID, publicID, newName string) (*domain.Project, error) {
	const q = `
UPDATE projects
SET name = $3, updated_at = now()
WHERE user_firebase_uid = $1 AND public_id = $2 AND deleted_at IS NULL
RETURNING public_id, name, is_temporary, created_at, updated_at;
`
	var p domain.Project
	err := r.db.QueryRowContext(ctx, q, userFirebaseUID, publicID, newName).
		Scan(&p.PublicID, &p.Name, &p.Temporary, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &p, nil
}

// SoftDelete marks a project as deleted (soft delete).
func (r *ProjectRepository) SoftDelete(ctx context.Context, userFirebaseUID, publicID string) (bool, error) {
	const q = `
UPDATE projects
SET deleted_at = now(), updated_at = now()
WHERE user_firebase_uid = $1 AND public_id = $2 AND deleted_at IS NULL;
`
	result, err := r.db.ExecContext(ctx, q, userFirebaseUID, publicID)
	if err != nil {
		return false, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rowsAffected > 0, nil
}
