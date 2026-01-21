package users

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct {
	db *pgxpool.Pool
}

func NewRepo(db *pgxpool.Pool) *Repo { return &Repo{db: db} }

type UpsertUser struct {
	FirebaseUID string
	Email       string
	DisplayName string
	PhotoURL    string
}

func (r *Repo) EnsureUser(ctx context.Context, u UpsertUser) (string, error) {
	fuid := strings.TrimSpace(u.FirebaseUID)
	if fuid == "" {
		return "", fmt.Errorf("firebase_uid required")
	}

	email := strings.TrimSpace(u.Email)
	if email == "" {
		// If your schema keeps EMAIL NOT NULL, you MUST provide it.
		// (Either enforce header X-User-Email or relax schema.)
		return "", fmt.Errorf("email required (send X-User-Email)")
	}

	const q = `
INSERT INTO users (firebase_uid, email, display_name, photo_url)
VALUES ($1, $2, NULLIF($3,''), NULLIF($4,''))
ON CONFLICT (firebase_uid) DO UPDATE
SET
  email = EXCLUDED.email,
  display_name = COALESCE(EXCLUDED.display_name, users.display_name),
  photo_url = COALESCE(EXCLUDED.photo_url, users.photo_url),
  updated_at = NOW()
RETURNING firebase_uid;
`
	var out string
	if err := r.db.QueryRow(ctx, q, fuid, email, u.DisplayName, u.PhotoURL).Scan(&out); err != nil {
		return "", err
	}
	return out, nil
}
