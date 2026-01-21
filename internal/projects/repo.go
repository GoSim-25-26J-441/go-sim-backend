package projects

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct {
	db *pgxpool.Pool
}

func NewRepo(db *pgxpool.Pool) *Repo {
	return &Repo{db: db}
}

type Project struct {
	PublicID  string    `json:"public_id"`
	Name      string    `json:"name"`
	Temporary bool      `json:"is_temporary"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (r *Repo) Create(ctx context.Context, userFirebaseUID, name string, temporary bool) (*Project, error) {
	if name == "" {
		return nil, fmt.Errorf("name required")
	}
	if userFirebaseUID == "" {
		return nil, fmt.Errorf("user firebase uid required")
	}

	for i := 0; i < 5; i++ {
		publicID, err := NewPublicID("archfind")
		if err != nil {
			return nil, err
		}

		const q = `
INSERT INTO projects (public_id, user_firebase_uid, name, is_temporary)
VALUES ($1, $2, $3, $4)
RETURNING public_id, name, is_temporary, created_at, updated_at;
`
		var p Project
		err = r.db.QueryRow(ctx, q, publicID, userFirebaseUID, name, temporary).
			Scan(&p.PublicID, &p.Name, &p.Temporary, &p.CreatedAt, &p.UpdatedAt)

		if err == nil {
			return &p, nil
		}

		// unique violation on public_id → retry
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			continue
		}
		return nil, err
	}

	return nil, fmt.Errorf("failed to generate unique project id")
}

func (r *Repo) List(ctx context.Context, userFirebaseUID string) ([]Project, error) {
	const q = `
SELECT public_id, name, is_temporary, created_at, updated_at
FROM projects
WHERE user_firebase_uid = $1 AND deleted_at IS NULL
ORDER BY created_at DESC;
`
	rows, err := r.db.Query(ctx, q, userFirebaseUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Project, 0, 16)
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.PublicID, &p.Name, &p.Temporary, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repo) Rename(ctx context.Context, userFirebaseUID, publicID, newName string) (*Project, error) {
	const q = `
UPDATE projects
SET name = $3, updated_at = now()
WHERE user_firebase_uid = $1 AND public_id = $2 AND deleted_at IS NULL
RETURNING public_id, name, is_temporary, created_at, updated_at;
`
	var p Project
	err := r.db.QueryRow(ctx, q, userFirebaseUID, publicID, newName).
		Scan(&p.PublicID, &p.Name, &p.Temporary, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *Repo) SoftDelete(ctx context.Context, userFirebaseUID, publicID string) (bool, error) {
	const q = `
UPDATE projects
SET deleted_at = now(), updated_at = now()
WHERE user_firebase_uid = $1 AND public_id = $2 AND deleted_at IS NULL;
`
	ct, err := r.db.Exec(ctx, q, userFirebaseUID, publicID)
	if err != nil {
		return false, err
	}
	return ct.RowsAffected() > 0, nil
}
