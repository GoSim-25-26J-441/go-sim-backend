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
	"gopkg.in/yaml.v3"
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

	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = fmt.Sprintf("diagramV%d", next)
	}

	diagramText := string(in.DiagramJSON)
	specText := ""
	if len(in.SpecSummary) > 0 {
		specText = string(in.SpecSummary)
	} else if diagramText != "" {
		if generated, err := generateSpecSummaryFromDiagram(diagramText); err == nil && generated != "" {
			specText = generated
		}
	}

	// Generate YAML content from spec_summary (if available).
	yamlText := ""
	if specText != "" {
		if y, err := generateYAMLFromSpecSummary(specText); err == nil {
			yamlText = y
		}
	}

	var ver domain.DiagramVersion
	ver.ID = id
	ver.ProjectPublicID = projectPublicID
	ver.UserFirebaseUID = userFirebaseUID
	ver.VersionNumber = next
	ver.Title = title
	ver.Source = in.Source
	ver.Hash = in.Hash
	ver.ImageObjectKey = in.ImageObjectKey
	ver.DiagramJSON = in.DiagramJSON
	ver.SpecSummary = json.RawMessage([]byte(specText))
	ver.YAMLContent = yamlText

	err = tx.QueryRowContext(ctx, `
insert into diagram_versions (
  id, project_public_id, user_firebase_uid,
  version_number, source, diagram_json, image_object_key, spec_summary, hash, created_by, title, yaml_content
)
values (
  $1, $2, $3,
  $4, $5,
  $6::jsonb,
  nullif($7,''),
  nullif($8,'')::jsonb,
  nullif($9,''),
  $10,
  $11,
  nullif($12,'')
)
returning created_at
`, id, projectPublicID, userFirebaseUID,
		next, in.Source,
		diagramText,
		in.ImageObjectKey,
		specText,
		in.Hash,
		userFirebaseUID,
		title,
		yamlText,
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

// generateSpecSummaryFromDiagram builds a minimal spec_summary JSON from the diagram_json payload.
// Expected diagram_json shape:
//
//	{
//	  "nodes": [{ "id": "...", "type": "service|db|...", "label": "..." }, ...],
//	  "edges": [{ "from": "node-id", "to": "node-id", "protocol": "REST|SQL|..." }, ...]
//	}
//
// Output spec_summary JSON:
//
//	{
//	  "services": ["service-label-1", "service-label-2", ...],
//	  "datastores": ["db-label-1", ...],
//	  "dependencies": ["fromLabel->toLabel(protocol)", ...]
//	}
func generateSpecSummaryFromDiagram(diagramText string) (string, error) {
	if strings.TrimSpace(diagramText) == "" {
		return "", nil
	}

	var payload struct {
		Nodes []struct {
			ID    string `json:"id"`
			Type  string `json:"type"`
			Label string `json:"label"`
		} `json:"nodes"`
		Edges []struct {
			From     string `json:"from"`
			To       string `json:"to"`
			Protocol string `json:"protocol"`
		} `json:"edges"`
	}
	if err := json.Unmarshal([]byte(diagramText), &payload); err != nil {
		return "", err
	}

	idToLabel := make(map[string]string, len(payload.Nodes))
	servicesSet := map[string]struct{}{}
	datastoresSet := map[string]struct{}{}

	for _, n := range payload.Nodes {
		lbl := strings.TrimSpace(n.Label)
		if lbl == "" {
			continue
		}
		idToLabel[n.ID] = lbl

		switch strings.ToLower(strings.TrimSpace(n.Type)) {
		case "db", "database", "datastore":
			datastoresSet[lbl] = struct{}{}
		default:
			servicesSet[lbl] = struct{}{}
		}
	}

	var services []string
	for s := range servicesSet {
		services = append(services, s)
	}
	var datastores []string
	for d := range datastoresSet {
		datastores = append(datastores, d)
	}

	var deps []string
	for _, e := range payload.Edges {
		fromLabel, okFrom := idToLabel[e.From]
		toLabel, okTo := idToLabel[e.To]
		if !okFrom || !okTo {
			continue
		}
		proto := strings.ToLower(strings.TrimSpace(e.Protocol))
		if proto == "" {
			proto = "unknown"
		}
		deps = append(deps, fmt.Sprintf("%s->%s(%s)", fromLabel, toLabel, proto))
	}

	out := map[string]interface{}{
		"services":     services,
		"datastores":   datastores,
		"dependencies": deps,
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// generateYAMLFromSpecSummary converts the flat spec_summary JSON into the full YAML structure.
// Input spec_summary JSON (what we store in spec_summary column):
//
//	{
//	  "services": ["user-1", "service-1"],
//	  "datastores": ["db-1"],
//	  "dependencies": ["user-1->service-1(rest)"]
//	}
//
// Output YAML (what we store in yaml_content): apis, configs, conflicts, constraints,
// datastores: [], dependencies, deploymentHints, gaps, metadata, services (including
// type: database for former datastores), topics, trace.
func generateYAMLFromSpecSummary(specText string) (string, error) {
	if strings.TrimSpace(specText) == "" {
		return "", nil
	}

	// SpecSummaryJSON mirrors the JSON we store in spec_summary.
	type SpecSummaryJSON struct {
		Services     []string `json:"services"`
		Datastores   []string `json:"datastores"`
		Dependencies []string `json:"dependencies"`
	}

	var ss SpecSummaryJSON
	if err := json.Unmarshal([]byte(specText), &ss); err != nil {
		return "", err
	}

	type yamlService struct {
		Name string `yaml:"name"`
		Type string `yaml:"type"`
	}
	type yamlDependency struct {
		From string `yaml:"from"`
		To   string `yaml:"to"`
		Kind string `yaml:"kind"`
		Sync bool   `yaml:"sync"`
	}
	type yamlAPI struct {
		Name     string `yaml:"name"`
		Protocol string `yaml:"protocol"`
	}

	// Full YAML structure: databases go under services with type: database; datastores stays empty.
	type fullYAMLSpec struct {
		APIs            []yamlAPI        `yaml:"apis"`
		Configs          map[string]any       `yaml:"configs"`
		Conflicts        []any                `yaml:"conflicts"`
		Constraints      map[string]any       `yaml:"constraints"`
		Datastores       []any                `yaml:"datastores"`
		Dependencies     []yamlDependency     `yaml:"dependencies"`
		DeploymentHints  map[string]any      `yaml:"deploymentHints"`
		Gaps             []any                `yaml:"gaps"`
		Metadata         map[string]any       `yaml:"metadata"`
		Services         []yamlService        `yaml:"services"`
		Topics           []any                `yaml:"topics"`
		Trace            []any                `yaml:"trace"`
	}

	ys := fullYAMLSpec{
		APIs:            []yamlAPI{{Name: "REST", Protocol: "rest"}},
		Configs:         map[string]any{"slo": map[string]any{"target_rps": 180}},
		Conflicts:       []any{},
		Constraints:     map[string]any{},
		Datastores:      []any{},
		Dependencies:    nil,
		DeploymentHints: map[string]any{},
		Gaps:            []any{},
		Metadata:        map[string]any{"generator": "sample", "schemaVersion": "0.1.0"},
		Services:        nil,
		Topics:          []any{},
		Trace:           []any{},
	}

	// Services first (type: service), then former datastores (type: database).
	for _, s := range ss.Services {
		name := strings.TrimSpace(s)
		if name == "" {
			continue
		}
		ys.Services = append(ys.Services, yamlService{Name: name, Type: "service"})
	}
	for _, d := range ss.Datastores {
		name := strings.TrimSpace(d)
		if name == "" {
			continue
		}
		ys.Services = append(ys.Services, yamlService{Name: name, Type: "database"})
	}

	// Parse dependencies "from->to(kind)".
	for _, d := range ss.Dependencies {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		from := ""
		to := ""
		kind := "rest"
		parts := strings.SplitN(d, "->", 2)
		if len(parts) == 2 {
			from = strings.TrimSpace(parts[0])
			right := strings.TrimSpace(parts[1])
			if idx := strings.Index(right, "("); idx >= 0 {
				to = strings.TrimSpace(right[:idx])
				if j := strings.Index(right[idx+1:], ")"); j >= 0 {
					kindPart := right[idx+1 : idx+1+j]
					if k := strings.TrimSpace(kindPart); k != "" {
						kind = strings.ToLower(k)
					}
				}
			} else {
				to = right
			}
		}
		if from == "" || to == "" {
			continue
		}
		ys.Dependencies = append(ys.Dependencies, yamlDependency{
			From: from,
			To:   to,
			Kind: kind,
			Sync: true,
		})
	}

	out, err := yaml.Marshal(ys)
	if err != nil {
		return "", err
	}
	return string(out), nil
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
select id, version_number, title, source,
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
		&ver.ID, &ver.VersionNumber, &ver.Title, &ver.Source,
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

// ListAllVersions returns all diagram versions for a project, ordered by version_number DESC
func (r *DiagramRepository) ListAllVersions(ctx context.Context, userFirebaseUID, projectPublicID string) ([]domain.DiagramVersion, error) {
	if strings.TrimSpace(userFirebaseUID) == "" {
		return nil, fmt.Errorf("user firebase uid required")
	}
	if strings.TrimSpace(projectPublicID) == "" {
		return nil, fmt.Errorf("project public_id required")
	}

	// Verify project exists and belongs to user
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

	const q = `
select id, version_number, title, source,
       coalesce(hash,''), coalesce(image_object_key,''),
       diagram_json::text,
       coalesce(spec_summary::text,''),
       created_at
from diagram_versions
where project_public_id = $1
  and user_firebase_uid = $2
order by version_number desc
`
	rows, err := r.db.QueryContext(ctx, q, projectPublicID, userFirebaseUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []domain.DiagramVersion
	for rows.Next() {
		var ver domain.DiagramVersion
		ver.ProjectPublicID = projectPublicID
		ver.UserFirebaseUID = userFirebaseUID

		var diagramText string
		var specText string

		if err := rows.Scan(
			&ver.ID, &ver.VersionNumber, &ver.Title, &ver.Source,
			&ver.Hash, &ver.ImageObjectKey,
			&diagramText, &specText,
			&ver.CreatedAt,
		); err != nil {
			return nil, err
		}

		if diagramText != "" {
			ver.DiagramJSON = json.RawMessage([]byte(diagramText))
		}
		if specText != "" {
			ver.SpecSummary = json.RawMessage([]byte(specText))
		}

		versions = append(versions, ver)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return versions, nil
}

// UpdateTitle updates the title of a diagram version for a specific user and project.
func (r *DiagramRepository) UpdateTitle(ctx context.Context, userFirebaseUID, projectPublicID, versionID, title string) (bool, error) {
	if strings.TrimSpace(userFirebaseUID) == "" {
		return false, fmt.Errorf("user firebase uid required")
	}
	if strings.TrimSpace(projectPublicID) == "" {
		return false, fmt.Errorf("project public_id required")
	}
	if strings.TrimSpace(versionID) == "" {
		return false, fmt.Errorf("version id required")
	}

	const q = `
update diagram_versions
set title = $1
where id = $2
  and project_public_id = $3
  and user_firebase_uid = $4
`
	res, err := r.db.ExecContext(ctx, q, strings.TrimSpace(title), versionID, projectPublicID, userFirebaseUID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
