package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/projects/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/projects/utils"
)

// DiagramRepository provides persistence operations for diagrams
type DiagramRepository struct {
	db *sql.DB
}

// NewDiagramRepository creates a new diagram repository
func NewDiagramRepository(db *sql.DB) *DiagramRepository {
	return &DiagramRepository{db: db}
}

func (r *DiagramRepository) CreateVersion(ctx context.Context, userFirebaseUID, projectPublicID string, in domain.CreateVersionInput) (*domain.DiagramVersion, error) {
	if strings.TrimSpace(userFirebaseUID) == "" {
		return nil, fmt.Errorf("user firebase uid required")
	}
	if strings.TrimSpace(projectPublicID) == "" {
		return nil, fmt.Errorf("project public_id required")
	}
	if len(in.DiagramJSON) == 0 {
		return nil, fmt.Errorf("diagram_json required")
	}
	if strings.TrimSpace(in.Source) == "" {
		in.Source = "canvas_json"
	}

	id, err := utils.NewTextID("dver")
	if err != nil {
		return nil, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var ok string
	err = tx.QueryRowContext(ctx, `
select public_id
from projects
where public_id = $1
  and user_firebase_uid = $2
  and deleted_at is null
for update
`, projectPublicID, userFirebaseUID).Scan(&ok)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}

	var next int
	if err := tx.QueryRowContext(ctx, `
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

	var ver domain.DiagramVersion
	ver.ID = id
	ver.ProjectPublicID = projectPublicID
	ver.UserFirebaseUID = userFirebaseUID
	ver.VersionNumber = next
	ver.Source = in.Source
	ver.Hash = in.Hash
	ver.ImageObjectKey = in.ImageObjectKey
	ver.DiagramJSON = in.DiagramJSON
	ver.SpecSummary = in.SpecSummary

	err = tx.QueryRowContext(ctx, `
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

	_, err = tx.ExecContext(ctx, `
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

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &ver, nil
}

func (r *DiagramRepository) Latest(ctx context.Context, userFirebaseUID, projectPublicID string) (*domain.DiagramVersion, error) {
	if strings.TrimSpace(userFirebaseUID) == "" {
		return nil, fmt.Errorf("user firebase uid required")
	}
	if strings.TrimSpace(projectPublicID) == "" {
		return nil, fmt.Errorf("project public_id required")
	}

	var ok string
	err := r.db.QueryRowContext(ctx, `
select public_id
from projects
where public_id = $1
  and user_firebase_uid = $2
  and deleted_at is null
`, projectPublicID, userFirebaseUID).Scan(&ok)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}

	var ver domain.DiagramVersion
	ver.ProjectPublicID = projectPublicID
	ver.UserFirebaseUID = userFirebaseUID

	var diagramText string
	var specText string

	err = r.db.QueryRowContext(ctx, `
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
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
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
