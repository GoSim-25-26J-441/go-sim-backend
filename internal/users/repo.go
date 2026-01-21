package users

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct {
	db *pgxpool.Pool
}

func NewRepo(db *pgxpool.Pool) *Repo {
	return &Repo{db: db}
}

type UpsertUser struct {
	FirebaseUID string
	Email       string
	DisplayName string
	PhotoURL    string
}

func (r *Repo) EnsureUser(ctx context.Context, u UpsertUser) (string, error) {
	if u.FirebaseUID == "" {
		return "", fmt.Errorf("firebase_uid required")
	}

	const q = `
insert into users (firebase_uid, email, display_name, photo_url, updated_at)
values ($1, nullif($2,''), nullif($3,''), nullif($4,''), now())
on conflict (firebase_uid) do update
set
  email = coalesce(excluded.email, users.email),
  display_name = coalesce(excluded.display_name, users.display_name),
  photo_url = coalesce(excluded.photo_url, users.photo_url),
  updated_at = now()
returning id::text;
`
	var id string
	if err := r.db.QueryRow(ctx, q, u.FirebaseUID, u.Email, u.DisplayName, u.PhotoURL).Scan(&id); err != nil {
		return "", err
	}
	return id, nil
}
