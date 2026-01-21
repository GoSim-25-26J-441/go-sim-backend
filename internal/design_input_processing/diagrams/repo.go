package diagrams

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type Repo struct {
	db *pgxpool.Pool
}

func NewRepo(db *pgxpool.Pool) *Repo {
	return &Repo{db: db}
}

type CreateVersionInput struct {
	Source         string
	DiagramJSON    json.RawMessage
	ImageObjectKey string
	SpecSummary    json.RawMessage
	Hash           string
}

type DiagramVersion struct {
	ID              string          `json:"id"`
	ProjectPublicID string          `json:"project_public_id"`
	UserFirebaseUID string          `json:"user_firebase_uid"`
	VersionNumber   int             `json:"version_number"`
	Source          string          `json:"source"`
	Hash            string          `json:"hash,omitempty"`
	ImageObjectKey  string          `json:"image_object_key,omitempty"`
	DiagramJSON     json.RawMessage `json:"diagram_json,omitempty"`
	SpecSummary     json.RawMessage `json:"spec_summary,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
}

func (r *Repo) CreateVersion(ctx context.Context, userFirebaseUID, projectPublicID string, in CreateVersionInput) (*DiagramVersion, error) {
	if stringsTrim(userFirebaseUID) == "" {
		return nil, fmt.Errorf("user firebase uid required")
	}
	if stringsTrim(projectPublicID) == "" {
		return nil, fmt.Errorf("project public_id required")
	}
	if len(in.DiagramJSON) == 0 {
		return nil, fmt.Errorf("diagram_json required")
	}
	if stringsTrim(in.Source) == "" {
		in.Source = "canvas_json"
	}

	id, err := newTextID("dver")
	if err != nil {
		return nil, err
	}

	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var ok string
	err = tx.QueryRow(ctx, `
select public_id
from projects
where public_id = $1
  and user_firebase_uid = $2
  and deleted_at is null
for update
`, projectPublicID, userFirebaseUID).Scan(&ok)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	var next int
	if err := tx.QueryRow(ctx, `
select coalesce(max(version_number), 0) + 1
from diagram_versions
where project_public_id = $1
  and user_firebase_uid = $2
`, projectPublicID, userFirebaseUID).Scan(&next); err != nil {
		return nil, err
	}

	diagramText := string(in.DiagramJSON)
	specText := ""
	if len(in.SpecSummary) > 0 {
		specText = string(in.SpecSummary)
	}

	var ver DiagramVersion
	ver.ID = id
	ver.ProjectPublicID = projectPublicID
	ver.UserFirebaseUID = userFirebaseUID
	ver.VersionNumber = next
	ver.Source = in.Source
	ver.Hash = in.Hash
	ver.ImageObjectKey = in.ImageObjectKey
	ver.DiagramJSON = in.DiagramJSON
	ver.SpecSummary = in.SpecSummary

	err = tx.QueryRow(ctx, `
insert into diagram_versions (
  id, project_public_id, user_firebase_uid,
  version_number, source, diagram_json, image_object_key, spec_summary, hash, created_by
)
values (
  $1, $2, $3,
  $4, $5,
  $6::jsonb,
  nullif($7,''),
  nullif($8,'')::jsonb,
  nullif($9,''),
  $10
)
returning created_at
`, id, projectPublicID, userFirebaseUID,
		next, in.Source,
		diagramText,
		in.ImageObjectKey,
		specText,
		in.Hash,
		userFirebaseUID,
	).Scan(&ver.CreatedAt)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(ctx, `
update projects
set current_diagram_version_id = $1,
    updated_at = now()
where public_id = $2
  and user_firebase_uid = $3
  and deleted_at is null
`, id, projectPublicID, userFirebaseUID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &ver, nil
}

func (r *Repo) Latest(ctx context.Context, userFirebaseUID, projectPublicID string) (*DiagramVersion, error) {
	if stringsTrim(userFirebaseUID) == "" {
		return nil, fmt.Errorf("user firebase uid required")
	}
	if stringsTrim(projectPublicID) == "" {
		return nil, fmt.Errorf("project public_id required")
	}

	var ok string
	err := r.db.QueryRow(ctx, `
select public_id
from projects
where public_id = $1
  and user_firebase_uid = $2
  and deleted_at is null
`, projectPublicID, userFirebaseUID).Scan(&ok)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	var ver DiagramVersion
	ver.ProjectPublicID = projectPublicID
	ver.UserFirebaseUID = userFirebaseUID

	var diagramText string
	var specText string

	err = r.db.QueryRow(ctx, `
select id, version_number, source,
       coalesce(hash,''), coalesce(image_object_key,''),
       diagram_json::text,
       coalesce(spec_summary::text,''),
       created_at
from diagram_versions
where project_public_id = $1
  and user_firebase_uid = $2
order by version_number desc
limit 1
`, projectPublicID, userFirebaseUID).Scan(
		&ver.ID, &ver.VersionNumber, &ver.Source,
		&ver.Hash, &ver.ImageObjectKey,
		&diagramText, &specText,
		&ver.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if diagramText != "" {
		ver.DiagramJSON = json.RawMessage([]byte(diagramText))
	}
	if specText != "" {
		ver.SpecSummary = json.RawMessage([]byte(specText))
	}

	return &ver, nil
}

func stringsTrim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\n' || s[0] == '\t' || s[0] == '\r') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\n' || s[len(s)-1] == '\t' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

func newTextID(prefix string) (string, error) {
	a, err := randInt(10000, 99999)
	if err != nil {
		return "", err
	}
	b, err := randInt(1000, 9999)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%05d-%04d", prefix, a, b), nil
}

func randInt(min, max int64) (int64, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(max-min+1))
	if err != nil {
		return 0, err
	}
	return min + n.Int64(), nil
}
