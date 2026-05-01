package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrDiagramVersionNotFound = errors.New("diagram version not found for project/user")
	ErrScenarioCacheConflict  = errors.New("scenario cache conflict")
	ErrDiagramMissingYAML     = errors.New("diagram version has no stored AMG/APD YAML")
)

type CachedScenario struct {
	DiagramVersionID string
	ScenarioYAML     string
	ScenarioHash     string
	S3Path           string
	Source           string
	SourceHash       string // SHA-256 hex of generation source (AMG/APD YAML, optionally combined with host config)
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type ScenarioCacheRepository struct {
	db *sql.DB
}

func NewScenarioCacheRepository(db *sql.DB) *ScenarioCacheRepository {
	return &ScenarioCacheRepository{db: db}
}

func hashScenarioYAML(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// HashAMGAPDSource returns SHA-256 hex of the AMG/APD diagram YAML bytes.
func HashAMGAPDSource(amgYAML string) string {
	return hashScenarioYAML(amgYAML)
}

// HashScenarioGenerationSource hashes diagram YAML plus optional canonical host-config JSON.
// When hostConfigCanonicalJSON is empty, the result equals HashAMGAPDSource(amgYAML) for backward compatibility.
func HashScenarioGenerationSource(amgYAML, hostConfigCanonicalJSON string) string {
	if strings.TrimSpace(hostConfigCanonicalJSON) == "" {
		return HashAMGAPDSource(amgYAML)
	}
	h := sha256.Sum256([]byte(amgYAML + "\n---host-config---\n" + hostConfigCanonicalJSON))
	return hex.EncodeToString(h[:])
}

func (r *ScenarioCacheRepository) ResolveCurrentDiagramVersionID(ctx context.Context, userID, projectID string) (string, error) {
	if r == nil || r.db == nil {
		return "", fmt.Errorf("scenario cache repository not initialized")
	}
	var id sql.NullString
	err := r.db.QueryRowContext(ctx, `
SELECT current_diagram_version_id
FROM projects
WHERE public_id = $1
  AND user_firebase_uid = $2
  AND deleted_at IS NULL
`, projectID, userID).Scan(&id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrDiagramVersionNotFound
		}
		return "", err
	}
	if !id.Valid || strings.TrimSpace(id.String) == "" {
		return "", ErrDiagramVersionNotFound
	}
	return id.String, nil
}

func (r *ScenarioCacheRepository) VerifyDiagramVersionForProject(ctx context.Context, userID, projectID, diagramVersionID string) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("scenario cache repository not initialized")
	}
	var id string
	err := r.db.QueryRowContext(ctx, `
SELECT id
FROM diagram_versions
WHERE id = $1
  AND project_public_id = $2
  AND user_firebase_uid = $3
`, diagramVersionID, projectID, userID).Scan(&id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrDiagramVersionNotFound
		}
		return err
	}
	return nil
}

// GetDiagramYAMLContent returns yaml_content for a diagram version scoped to user/project.
func (r *ScenarioCacheRepository) GetDiagramYAMLContent(ctx context.Context, userID, projectID, diagramVersionID string) (string, error) {
	if r == nil || r.db == nil {
		return "", fmt.Errorf("scenario cache repository not initialized")
	}
	var content sql.NullString
	err := r.db.QueryRowContext(ctx, `
SELECT yaml_content
FROM diagram_versions
WHERE id = $1
  AND project_public_id = $2
  AND user_firebase_uid = $3
`, diagramVersionID, projectID, userID).Scan(&content)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrDiagramVersionNotFound
		}
		return "", err
	}
	if !content.Valid || strings.TrimSpace(content.String) == "" {
		return "", ErrDiagramMissingYAML
	}
	return content.String, nil
}

func (r *ScenarioCacheRepository) GetScenarioForDiagramVersion(ctx context.Context, userID, projectID, diagramVersionID string) (*CachedScenario, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("scenario cache repository not initialized")
	}
	var c CachedScenario
	var s3, srcHash sql.NullString
	err := r.db.QueryRowContext(ctx, `
SELECT c.diagram_version_id, c.scenario_yaml, c.scenario_hash, c.s3_path, c.source, c.source_hash, c.created_at, c.updated_at
FROM simulation_scenario_cache c
JOIN diagram_versions dv ON dv.id = c.diagram_version_id
WHERE dv.id = $1
  AND dv.project_public_id = $2
  AND dv.user_firebase_uid = $3
`, diagramVersionID, projectID, userID).Scan(
		&c.DiagramVersionID, &c.ScenarioYAML, &c.ScenarioHash, &s3, &c.Source, &srcHash, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if s3.Valid {
		c.S3Path = s3.String
	}
	if srcHash.Valid {
		c.SourceHash = srcHash.String
	}
	return &c, nil
}

// UpsertScenarioForDiagramVersion persists scenario YAML for a diagram version.
// sourceHash is the SHA-256 hex of AMG/APD YAML when applicable; nil keeps existing source_hash on update.
func (r *ScenarioCacheRepository) UpsertScenarioForDiagramVersion(ctx context.Context, diagramVersionID, scenarioYAML, source, s3Path string, sourceHash *string, overwrite bool) (*CachedScenario, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("scenario cache repository not initialized")
	}
	if strings.TrimSpace(diagramVersionID) == "" {
		return nil, fmt.Errorf("diagram_version_id is required")
	}
	if scenarioYAML == "" {
		return nil, fmt.Errorf("scenario_yaml is required")
	}
	if strings.TrimSpace(source) == "" {
		source = "request"
	}
	newHash := hashScenarioYAML(scenarioYAML)

	var existing CachedScenario
	var existingS3, existingSrcHash sql.NullString
	err := r.db.QueryRowContext(ctx, `
SELECT diagram_version_id, scenario_yaml, scenario_hash, s3_path, source, source_hash, created_at, updated_at
FROM simulation_scenario_cache
WHERE diagram_version_id = $1
`, diagramVersionID).Scan(
		&existing.DiagramVersionID, &existing.ScenarioYAML, &existing.ScenarioHash, &existingS3, &existing.Source, &existingSrcHash, &existing.CreatedAt, &existing.UpdatedAt,
	)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil {
		if existingS3.Valid {
			existing.S3Path = existingS3.String
		}
		if existingSrcHash.Valid {
			existing.SourceHash = existingSrcHash.String
		}
		if existing.ScenarioHash == newHash {
			return &existing, nil
		}
		if !overwrite {
			return nil, ErrScenarioCacheConflict
		}
	}

	var sh interface{}
	if sourceHash != nil && *sourceHash != "" {
		sh = *sourceHash
	} else {
		sh = nil
	}

	var out CachedScenario
	var outS3, outSrcHash sql.NullString
	err = r.db.QueryRowContext(ctx, `
INSERT INTO simulation_scenario_cache (
  diagram_version_id, scenario_yaml, scenario_hash, s3_path, source, source_hash
) VALUES ($1, $2, $3, nullif($4,''), $5, $6)
ON CONFLICT (diagram_version_id) DO UPDATE
SET scenario_yaml = EXCLUDED.scenario_yaml,
    scenario_hash = EXCLUDED.scenario_hash,
    s3_path = COALESCE(EXCLUDED.s3_path, simulation_scenario_cache.s3_path),
    source = EXCLUDED.source,
    source_hash = COALESCE(EXCLUDED.source_hash, simulation_scenario_cache.source_hash),
    updated_at = NOW()
RETURNING diagram_version_id, scenario_yaml, scenario_hash, s3_path, source, source_hash, created_at, updated_at
`, diagramVersionID, scenarioYAML, newHash, s3Path, source, sh).Scan(
		&out.DiagramVersionID, &out.ScenarioYAML, &out.ScenarioHash, &outS3, &out.Source, &outSrcHash, &out.CreatedAt, &out.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if outS3.Valid {
		out.S3Path = outS3.String
	}
	if outSrcHash.Valid {
		out.SourceHash = outSrcHash.String
	}
	return &out, nil
}
