package diagrams

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	CreatedByUser  string
}

type DiagramVersion struct {
	ID              string          `json:"id"`
	ProjectPublicID string          `json:"project_public_id"`
	ProjectID       string          `json:"project_id"`
	VersionNumber   int             `json:"version_number"`
	Source          string          `json:"source"`
	Hash            string          `json:"hash,omitempty"`
	ImageObjectKey  string          `json:"image_object_key,omitempty"`
	DiagramJSON     json.RawMessage `json:"diagram_json,omitempty"`
	SpecSummary     json.RawMessage `json:"spec_summary,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
}

func (r *Repo) CreateVersion(ctx context.Context, userDBID, projectPublicID string, in CreateVersionInput) (*DiagramVersion, error) {
	if userDBID == "" {
		return nil, fmt.Errorf("user id required")
	}
	if projectPublicID == "" {
		return nil, fmt.Errorf("project public_id required")
	}
	if len(in.DiagramJSON) == 0 {
		return nil, fmt.Errorf("diagram_json required")
	}
	if in.Source == "" {
		in.Source = "canvas_json"
	}
	in.CreatedByUser = userDBID

	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var projectID string
	err = tx.QueryRow(ctx, `
select id::text
from projects
where public_id = $1 and user_id = $2::uuid and deleted_at is null
for update
`, projectPublicID, userDBID).Scan(&projectID)
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
where project_id = $1::uuid
`, projectID).Scan(&next); err != nil {
		return nil, err
	}

	diagramText := string(in.DiagramJSON)
	specText := ""
	if len(in.SpecSummary) > 0 {
		specText = string(in.SpecSummary)
	}

	var ver DiagramVersion
	ver.ProjectPublicID = projectPublicID
	ver.ProjectID = projectID
	ver.VersionNumber = next
	ver.Source = in.Source
	ver.Hash = in.Hash
	ver.ImageObjectKey = in.ImageObjectKey
	ver.DiagramJSON = in.DiagramJSON
	ver.SpecSummary = in.SpecSummary

	err = tx.QueryRow(ctx, `
insert into diagram_versions (
  project_id, version_number, source, diagram_json, image_object_key, spec_summary, hash, created_by
)
values (
  $1::uuid, $2, $3,
  $4::jsonb,
  nullif($5,''),
  nullif($6,'')::jsonb,
  nullif($7,''),
  $8::uuid
)
returning id::text, created_at
`, projectID, next, in.Source, diagramText, in.ImageObjectKey, specText, in.Hash, in.CreatedByUser).
		Scan(&ver.ID, &ver.CreatedAt)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(ctx, `
update projects
set current_diagram_version_id = $1::uuid,
    updated_at = now()
where id = $2::uuid
`, ver.ID, projectID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &ver, nil
}

func (r *Repo) Latest(ctx context.Context, userDBID, projectPublicID string) (*DiagramVersion, error) {
	if userDBID == "" {
		return nil, fmt.Errorf("user id required")
	}

	var projectID string
	err := r.db.QueryRow(ctx, `
select id::text
from projects
where public_id = $1 and user_id = $2::uuid and deleted_at is null
`, projectPublicID, userDBID).Scan(&projectID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	var ver DiagramVersion
	ver.ProjectPublicID = projectPublicID
	ver.ProjectID = projectID

	var diagramText string
	var specText string

	err = r.db.QueryRow(ctx, `
select id::text, version_number, source,
       coalesce(hash,''), coalesce(image_object_key,''),
       diagram_json::text,
       coalesce(spec_summary::text,''),
       created_at
from diagram_versions
where project_id = $1::uuid
order by version_number desc
limit 1
`, projectID).Scan(
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
