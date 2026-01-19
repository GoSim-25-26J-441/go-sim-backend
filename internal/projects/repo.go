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

func (r *Repo) Create(ctx context.Context, userDBID, name string, temporary bool) (*Project, error) {
	if name == "" {
		return nil, fmt.Errorf("name required")
	}

	for i := 0; i < 5; i++ {
		publicID, err := NewPublicID("archfind")
		if err != nil {
			return nil, err
		}

		const q = `
insert into projects (public_id, user_id, name, is_temporary)
values ($1, $2::uuid, $3, $4)
returning public_id, name, is_temporary, created_at, updated_at;
`
		var p Project
		err = r.db.QueryRow(ctx, q, publicID, userDBID, name, temporary).
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

func (r *Repo) List(ctx context.Context, userDBID string) ([]Project, error) {
	const q = `
select public_id, name, is_temporary, created_at, updated_at
from projects
where user_id = $1::uuid and deleted_at is null
order by created_at desc;
`
	rows, err := r.db.Query(ctx, q, userDBID)
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

func (r *Repo) Rename(ctx context.Context, userDBID, publicID, newName string) (*Project, error) {
	const q = `
update projects
set name = $3, updated_at = now()
where user_id = $1::uuid and public_id = $2 and deleted_at is null
returning public_id, name, is_temporary, created_at, updated_at;
`
	var p Project
	err := r.db.QueryRow(ctx, q, userDBID, publicID, newName).
		Scan(&p.PublicID, &p.Name, &p.Temporary, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *Repo) SoftDelete(ctx context.Context, userDBID, publicID string) (bool, error) {
	const q = `
update projects
set deleted_at = now(), updated_at = now()
where user_id = $1::uuid and public_id = $2 and deleted_at is null;
`
	ct, err := r.db.Exec(ctx, q, userDBID, publicID)
	if err != nil {
		return false, err
	}
	return ct.RowsAffected() > 0, nil
}
