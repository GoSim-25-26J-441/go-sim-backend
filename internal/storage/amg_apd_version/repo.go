package amg_apd_version

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/projects/utils"
)

const (
	DefaultUserID = "TestUser123"
	DefaultChatID = "TestChat123"
)

// VersionRow is a diagram_versions row loaded for AMG-APD APIs (any source).
type VersionRow struct {
	ID             string
	UserID         string
	ChatID         string
	VersionNumber  int
	Title          string
	Source         string
	YAMLContent    string
	GraphJSON      []byte
	DOTContent     string
	DetectionsJSON []byte
	CreatedAt      time.Time
}

// Repo persists AMG-APD analyses for versioning and compare.
type Repo struct {
	db *sql.DB
}

// diagramService represents one entry in the "services" array in diagram_json.
type diagramService struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// diagramDatastore represents one entry in the "datastores" array in diagram_json.
type diagramDatastore struct {
	Name string `json:"name"`
}

// diagramTopic represents one entry in the "topics" array in diagram_json.
type diagramTopic struct {
	Name string `json:"name"`
}

// diagramDependency represents one entry in the "dependencies" array in diagram_json.
type diagramDependency struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Kind  string `json:"kind"`
	Sync  bool   `json:"sync"`
	Label string `json:"label"`
}

// diagramEnvelope is the full JSON shape stored in diagram_versions.diagram_json.
// It keeps the original graph/detections fields and adds the requested structure.
type diagramEnvelope struct {
	Graph        json.RawMessage     `json:"graph,omitempty"`
	Detections   json.RawMessage     `json:"detections,omitempty"`
	Services     []diagramService    `json:"services"`
	Datastores   []diagramDatastore  `json:"datastores"`
	Topics       []diagramTopic      `json:"topics"`
	Dependencies []diagramDependency `json:"dependencies"`
}

// extractGraphAndDetectionsFromDiagramJSON supports both:
// 1) envelope: { graph, detections, ... }
// 2) legacy/raw graph: { nodes, edges }
func extractGraphAndDetectionsFromDiagramJSON(diagramJSON []byte) (graphJSON, detectionsJSON []byte) {
	if len(diagramJSON) == 0 {
		return nil, nil
	}

	var payload struct {
		Graph      json.RawMessage `json:"graph,omitempty"`
		Detections json.RawMessage `json:"detections,omitempty"`
		Nodes      json.RawMessage `json:"nodes,omitempty"`
		Edges      json.RawMessage `json:"edges,omitempty"`
	}
	if err := json.Unmarshal(diagramJSON, &payload); err != nil {
		return nil, nil
	}

	// Preferred shape: envelope
	if len(payload.Graph) > 0 {
		graphJSON = payload.Graph
		if len(payload.Detections) > 0 {
			detectionsJSON = payload.Detections
		}
		return graphJSON, detectionsJSON
	}

	// Fallback shape: raw graph object
	if len(payload.Nodes) > 0 && len(payload.Edges) > 0 {
		return diagramJSON, nil
	}
	return nil, nil
}

// buildDiagramEnvelope converts the analyzed graph JSON plus detections JSON into the
// combined envelope we persist in diagram_versions.diagram_json.
func buildDiagramEnvelope(graphJSON, detectionsJSON []byte) (*diagramEnvelope, error) {
	env := &diagramEnvelope{
		Services:     []diagramService{},
		Datastores:   []diagramDatastore{},
		Topics:       []diagramTopic{},
		Dependencies: []diagramDependency{},
	}

	if len(graphJSON) > 0 {
		env.Graph = json.RawMessage(graphJSON)
	}
	if len(detectionsJSON) > 0 {
		env.Detections = json.RawMessage(detectionsJSON)
	}

	// If there's no graph JSON, we still return the envelope with empty arrays
	// so the JSON structure is always present.
	if len(graphJSON) == 0 {
		return env, nil
	}

	var g domain.Graph
	if err := json.Unmarshal(graphJSON, &g); err != nil {
		// If the stored graph cannot be parsed, fall back to just graph/detections.
		return env, nil
	}

	// Map nodes → services / datastores / topics.
	for _, n := range g.Nodes {
		if n == nil {
			continue
		}
		switch n.Kind {
		case domain.NodeDB:
			env.Datastores = append(env.Datastores, diagramDatastore{
				Name: n.Name,
			})
		case domain.NodeAPIGateway:
			env.Services = append(env.Services, diagramService{
				Name: n.Name,
				Kind: "gateway",
			})
		case domain.NodeClient:
			env.Services = append(env.Services, diagramService{
				Name: n.Name,
				Kind: "client",
			})
		case domain.NodeUserActor:
			env.Services = append(env.Services, diagramService{
				Name: n.Name,
				Kind: "user_actor",
			})
		case domain.NodeExternalSystem:
			env.Services = append(env.Services, diagramService{
				Name: n.Name,
				Kind: "external_system",
			})
		case domain.NodeEventTopic:
			env.Topics = append(env.Topics, diagramTopic{
				Name: n.Name,
			})
		case domain.NodeService:
			env.Services = append(env.Services, diagramService{
				Name: n.Name,
				Kind: "service",
			})
		default:
			env.Services = append(env.Services, diagramService{
				Name: n.Name,
				Kind: "service",
			})
		}
	}

	// Map edges → dependencies.
	for _, e := range g.Edges {
		if e == nil {
			continue
		}

		fromName := e.From
		if n, ok := g.Nodes[e.From]; ok && n != nil && n.Name != "" {
			fromName = n.Name
		}
		toName := e.To
		if n, ok := g.Nodes[e.To]; ok && n != nil && n.Name != "" {
			toName = n.Name
		}

		kind := "rest"
		if e.Attrs != nil {
			if v, ok := e.Attrs["dep_kind"]; ok {
				if s, ok := v.(string); ok && s != "" {
					kind = s
				}
			}
		}

		syncVal := false
		if e.Attrs != nil {
			if v, ok := e.Attrs["sync"]; ok {
				if b, ok := v.(bool); ok {
					syncVal = b
				}
			}
		}

		env.Dependencies = append(env.Dependencies, diagramDependency{
			From:  fromName,
			To:    toName,
			Kind:  kind,
			Sync:  syncVal,
			Label: fmt.Sprintf("%s \u2192 %s", fromName, toName),
		})
	}

	return env, nil
}

func NewRepo(db *sql.DB) *Repo {
	return &Repo{db: db}
}

// alignDiagramVAutoTitle keeps default names in sync with the assigned version_number.
// Callers (e.g. the AMG-APD UI) may send "diagramV1" while the DB next row is v2 because
// version 1 is a canvas_json row not included in their AMG-only version list.
func alignDiagramVAutoTitle(title string, nextVersion int) string {
	t := strings.TrimSpace(title)
	if t == "" {
		return fmt.Sprintf("diagramV%d", nextVersion)
	}
	const pfx = "diagramV"
	if !strings.HasPrefix(t, pfx) {
		return t
	}
	numStr := strings.TrimPrefix(t, pfx)
	n, err := strconv.Atoi(numStr)
	if err != nil {
		return t
	}
	if n != nextVersion {
		return fmt.Sprintf("diagramV%d", nextVersion)
	}
	return t
}

// Save stores a new version for the given user_id and chat_id. version_number is auto-incremented per (user_id, chat_id).
func (r *Repo) Save(userID, chatID, title, yamlContent string, graphJSON, detectionsJSON []byte, dotContent string) (*VersionRow, error) {
	if userID == "" {
		userID = DefaultUserID
	}
	if chatID == "" {
		chatID = DefaultChatID
	}

	var nextVersion int
	err := r.db.QueryRow(`
		SELECT COALESCE(MAX(version_number), 0) + 1
		FROM diagram_versions
		WHERE user_firebase_uid = $1 AND project_public_id = $2
	`, userID, chatID).Scan(&nextVersion)
	if err != nil {
		return nil, err
	}

	title = alignDiagramVAutoTitle(title, nextVersion)

	// Build full diagram_json payload: graph, detections, and the structured
	// services/datastores/topics/dependencies JSON used by the simulator.
	env, err := buildDiagramEnvelope(graphJSON, detectionsJSON)
	if err != nil {
		return nil, err
	}
	diagramJSON, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}

	// Carry over diagram metadata from the latest existing version (any source), same as when
	// a diagram is saved from the canvas/chat flow — avoids empty image_object_key / spec_summary / created_by.
	var prevImg sql.NullString
	var prevSpec []byte
	var prevCreatedBy sql.NullString
	errPrev := r.db.QueryRow(`
		SELECT image_object_key, spec_summary, created_by
		FROM diagram_versions
		WHERE user_firebase_uid = $1 AND project_public_id = $2
		ORDER BY version_number DESC
		LIMIT 1
	`, userID, chatID).Scan(&prevImg, &prevSpec, &prevCreatedBy)
	if errPrev != nil && errPrev != sql.ErrNoRows {
		return nil, errPrev
	}

	createdBy := userID
	if prevCreatedBy.Valid && strings.TrimSpace(prevCreatedBy.String) != "" {
		createdBy = strings.TrimSpace(prevCreatedBy.String)
	}

	imgKey := ""
	if prevImg.Valid {
		imgKey = strings.TrimSpace(prevImg.String)
	}

	specJSON := ""
	if len(prevSpec) > 0 {
		specJSON = strings.TrimSpace(string(prevSpec))
	}

	id, err := utils.NewTextID("dver")
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`
		INSERT INTO diagram_versions (
			id,
			user_firebase_uid,
			project_public_id,
			version_number,
			source,
			title,
			yaml_content,
			diagram_json,
			dot_content,
			image_object_key,
			spec_summary,
			created_by
		)
		VALUES (
			$1, $2, $3, $4, 'amg_apd', $5, $6, $7, $8,
			NULLIF(TRIM($9), ''),
			CASE WHEN TRIM(COALESCE($10::text, '')) = '' THEN NULL ELSE $10::jsonb END,
			$11
		)
	`, id, userID, chatID, nextVersion, title, yamlContent, diagramJSON, dotContent, imgKey, specJSON, createdBy)
	if err != nil {
		return nil, err
	}

	// Keep project's current_diagram_version_id in sync so chat (FOLLOW_LATEST) uses this version.
	if chatID != "" && chatID != DefaultChatID {
		_, _ = r.db.Exec(`
			UPDATE projects
			SET current_diagram_version_id = $1, updated_at = now()
			WHERE user_firebase_uid = $2 AND public_id = $3 AND deleted_at IS NULL
		`, id, userID, chatID)
	}

	row := &VersionRow{
		ID:             id,
		UserID:         userID,
		ChatID:         chatID,
		VersionNumber:  nextVersion,
		Title:          title,
		Source:         "amg_apd",
		YAMLContent:    yamlContent,
		GraphJSON:      graphJSON,
		DOTContent:     dotContent,
		DetectionsJSON: detectionsJSON,
	}
	row.CreatedAt = time.Now().UTC()
	return row, nil
}

// ListByUserChat returns versions for the given user_id and chat_id, newest first.
func (r *Repo) ListByUserChat(userID, chatID string) ([]VersionRow, error) {
	if userID == "" {
		userID = DefaultUserID
	}
	if chatID == "" {
		chatID = DefaultChatID
	}

	rows, err := r.db.Query(`
		SELECT id, user_firebase_uid, project_public_id, version_number, title, yaml_content, diagram_json, dot_content, created_at
		FROM diagram_versions
		WHERE user_firebase_uid = $1 AND project_public_id = $2
		ORDER BY version_number DESC
	`, userID, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []VersionRow
	for rows.Next() {
		var row VersionRow
		var diagramJSON []byte
		var dotContent sql.NullString
		err := rows.Scan(&row.ID, &row.UserID, &row.ChatID, &row.VersionNumber, &row.Title,
			&row.YAMLContent, &diagramJSON, &dotContent, &row.CreatedAt)
		if err != nil {
			return nil, err
		}
		row.GraphJSON, row.DetectionsJSON = extractGraphAndDetectionsFromDiagramJSON(diagramJSON)
		if dotContent.Valid {
			row.DOTContent = dotContent.String
		}
		list = append(list, row)
	}
	return list, rows.Err()
}

// GetByID returns a single version by id. Returns nil if not found.
func (r *Repo) GetByID(id string) (*VersionRow, error) {
	row := &VersionRow{ID: id}
	var diagramJSON []byte
	var dotContent sql.NullString
	err := r.db.QueryRow(`
		SELECT user_firebase_uid, project_public_id, version_number, title, yaml_content, diagram_json, dot_content, created_at, source
		FROM diagram_versions
		WHERE id = $1
	`, id).Scan(&row.UserID, &row.ChatID, &row.VersionNumber, &row.Title,
		&row.YAMLContent, &diagramJSON, &dotContent, &row.CreatedAt, &row.Source)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	row.GraphJSON, row.DetectionsJSON = extractGraphAndDetectionsFromDiagramJSON(diagramJSON)
	if dotContent.Valid {
		row.DOTContent = dotContent.String
	}
	return row, nil
}

// GetByIDForUserChat returns a version by id only if it belongs to the given user_id and chat_id.
func (r *Repo) GetByIDForUserChat(id, userID, chatID string) (*VersionRow, error) {
	userID = strings.TrimSpace(userID)
	chatID = strings.TrimSpace(chatID)
	if userID == "" {
		userID = DefaultUserID
	}
	if chatID == "" {
		chatID = DefaultChatID
	}
	row, err := r.GetByID(id)
	if err != nil || row == nil {
		return row, err
	}
	if strings.TrimSpace(row.UserID) != userID || strings.TrimSpace(row.ChatID) != chatID {
		return nil, nil
	}
	return row, nil
}

// UpdateTitleForUserChat sets the display title for a version owned by the user/project (any source).
func (r *Repo) UpdateTitleForUserChat(id, userID, chatID, title string) (*VersionRow, error) {
	userID = strings.TrimSpace(userID)
	chatID = strings.TrimSpace(chatID)
	if userID == "" {
		userID = DefaultUserID
	}
	if chatID == "" {
		chatID = DefaultChatID
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}
	res, err := r.db.Exec(`
		UPDATE diagram_versions
		SET title = $1
		WHERE id = $2 AND user_firebase_uid = $3 AND project_public_id = $4
	`, title, id, userID, chatID)
	if err != nil {
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil
	}
	return r.GetByIDForUserChat(id, userID, chatID)
}

// DeleteByID deletes a version by id. Returns whether a row was deleted.
func (r *Repo) DeleteByID(id string) (bool, error) {
	res, err := r.db.Exec(`DELETE FROM diagram_versions WHERE id = $1 AND source = 'amg_apd'`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// DeleteByIDForUserChat deletes a version by id only if it belongs to user_id and chat_id.
func (r *Repo) DeleteByIDForUserChat(id, userID, chatID string) (bool, error) {
	userID = strings.TrimSpace(userID)
	chatID = strings.TrimSpace(chatID)
	if userID == "" {
		userID = DefaultUserID
	}
	if chatID == "" {
		chatID = DefaultChatID
	}
	res, err := r.db.Exec(`DELETE FROM diagram_versions WHERE id = $1 AND user_firebase_uid = $2 AND project_public_id = $3 AND source = 'amg_apd'`, id, userID, chatID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// DeleteByProject deletes all AMG-APD versions for the given user and project (e.g. when project is deleted).
func (r *Repo) DeleteByProject(userID, projectPublicID string) (int64, error) {
	if userID == "" {
		userID = DefaultUserID
	}
	if projectPublicID == "" {
		return 0, nil
	}
	res, err := r.db.Exec(`
		DELETE FROM diagram_versions
		WHERE user_firebase_uid = $1 AND project_public_id = $2 AND source = 'amg_apd'
	`, userID, projectPublicID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// VersionSummary is a lightweight summary for list responses.
type VersionSummary struct {
	ID            string    `json:"id"`
	VersionNumber int       `json:"version_number"`
	Title         string    `json:"title"`
	Source        string    `json:"source,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// ListSummariesByUserChat returns lightweight summaries for user/chat (all diagram_versions
// for the project, not only source = amg_apd, so the main canvas row appears alongside AMG saves).
func (r *Repo) ListSummariesByUserChat(userID, chatID string) ([]VersionSummary, error) {
	userID = strings.TrimSpace(userID)
	chatID = strings.TrimSpace(chatID)
	if userID == "" {
		userID = DefaultUserID
	}
	if chatID == "" {
		chatID = DefaultChatID
	}
	rows, err := r.db.Query(`
		SELECT id, version_number, title, source, created_at
		FROM diagram_versions
		WHERE user_firebase_uid = $1 AND project_public_id = $2
		ORDER BY version_number DESC
	`, userID, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []VersionSummary
	for rows.Next() {
		var s VersionSummary
		if err := rows.Scan(&s.ID, &s.VersionNumber, &s.Title, &s.Source, &s.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, rows.Err()
}

// GetLatestByUserProject returns the latest AMG-APD version for a given user and project_public_id.
func (r *Repo) GetLatestByUserProject(userID, projectPublicID string) (*VersionRow, error) {
	if userID == "" {
		userID = DefaultUserID
	}
	if projectPublicID == "" {
		projectPublicID = DefaultChatID
	}

	row := &VersionRow{}
	var diagramJSON []byte
	var dotContent sql.NullString
	err := r.db.QueryRow(`
		SELECT id, user_firebase_uid, project_public_id, version_number, title, yaml_content, diagram_json, dot_content, created_at, source
		FROM diagram_versions
		WHERE user_firebase_uid = $1 AND project_public_id = $2 AND source = 'amg_apd'
		ORDER BY version_number DESC
		LIMIT 1
	`, userID, projectPublicID).Scan(
		&row.ID, &row.UserID, &row.ChatID, &row.VersionNumber, &row.Title,
		&row.YAMLContent, &diagramJSON, &dotContent, &row.CreatedAt, &row.Source,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	row.GraphJSON, row.DetectionsJSON = extractGraphAndDetectionsFromDiagramJSON(diagramJSON)
	if dotContent.Valid {
		row.DOTContent = dotContent.String
	}
	return row, nil
}

// GetLatestYAMLByUserProject returns yaml_content and title from the latest diagram_versions
// row (any source) for the given user and project that has non-empty yaml_content.
// Used when no AMG-APD version exists yet, to run analysis from project/diagram YAML.
func (r *Repo) GetLatestYAMLByUserProject(userID, projectPublicID string) (yamlContent, title string, err error) {
	id, yaml, t, err := r.GetLatestDiagramRowByUserProject(userID, projectPublicID)
	if err != nil || id == "" {
		return "", "", err
	}
	_ = id
	if yaml != "" {
		if t != "" {
			title = t
		} else {
			title = "Uploaded"
		}
		return yaml, title, nil
	}
	return "", "", nil
}

// GetLatestDiagramRowByUserProject returns id, yaml_content, title of the latest diagram_versions
// row (any source) for the given user and project that has non-empty yaml_content.
func (r *Repo) GetLatestDiagramRowByUserProject(userID, projectPublicID string) (id, yamlContent, title string, err error) {
	if userID == "" {
		userID = DefaultUserID
	}
	if projectPublicID == "" {
		projectPublicID = DefaultChatID
	}
	var yaml, t sql.NullString
	err = r.db.QueryRow(`
		SELECT id, yaml_content, title
		FROM diagram_versions
		WHERE user_firebase_uid = $1 AND project_public_id = $2
		  AND yaml_content IS NOT NULL AND trim(yaml_content) != ''
		ORDER BY version_number DESC
		LIMIT 1
	`, userID, projectPublicID).Scan(&id, &yaml, &t)
	if err == sql.ErrNoRows {
		return "", "", "", nil
	}
	if err != nil {
		return "", "", "", err
	}
	if yaml.Valid && yaml.String != "" {
		if t.Valid && t.String != "" {
			title = t.String
		} else {
			title = "Uploaded"
		}
		return id, yaml.String, title, nil
	}
	return "", "", "", nil
}

// UpdateAnalysisByID updates the stored analysis fields for an existing AMG-APD version row.
// This does NOT create a new version; it overwrites diagram_json/dot_content for the given id,
// scoped to the given user_id + project_public_id.
func (r *Repo) UpdateAnalysisByID(id, userID, projectPublicID string, graphJSON, detectionsJSON []byte, dotContent string) error {
	if userID == "" {
		userID = DefaultUserID
	}
	if projectPublicID == "" {
		projectPublicID = DefaultChatID
	}

	// Rebuild the full diagram_json envelope with the new analysis.
	env, err := buildDiagramEnvelope(graphJSON, detectionsJSON)
	if err != nil {
		return err
	}
	diagramJSON, err := json.Marshal(env)
	if err != nil {
		return err
	}

	res, err := r.db.Exec(`
		UPDATE diagram_versions
		SET diagram_json = $1,
		    dot_content = $2
		WHERE id = $3
		  AND user_firebase_uid = $4
		  AND project_public_id = $5
		  AND source = 'amg_apd'
	`, diagramJSON, dotContent, id, userID, projectPublicID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpdateDiagramVersionAnalysisByID updates diagram_json and dot_content for the given
// diagram_versions row (any source). Version 1 keeps its existing source (e.g. canvas_json
// from the main canvas); version 2+ are marked source = 'amg_apd' when analysis is written
// from the AMG-APD flow.
func (r *Repo) UpdateDiagramVersionAnalysisByID(id, userID, projectPublicID string, graphJSON, detectionsJSON []byte, dotContent string) error {
	if userID == "" {
		userID = DefaultUserID
	}
	if projectPublicID == "" {
		projectPublicID = DefaultChatID
	}
	env, err := buildDiagramEnvelope(graphJSON, detectionsJSON)
	if err != nil {
		return err
	}
	diagramJSON, err := json.Marshal(env)
	if err != nil {
		return err
	}
	res, err := r.db.Exec(`
		UPDATE diagram_versions
		SET diagram_json = $1,
		    dot_content = $2,
		    source = CASE WHEN version_number = 1 THEN source ELSE 'amg_apd' END
		WHERE id = $3 AND user_firebase_uid = $4 AND project_public_id = $5
	`, diagramJSON, dotContent, id, userID, projectPublicID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// GetByIDForUserProject returns a diagram_versions row by id and user+project (any source).
// Used by update-version-analysis to load the row to update.
func (r *Repo) GetByIDForUserProject(id, userID, projectPublicID string) (*VersionRow, error) {
	if userID == "" {
		userID = DefaultUserID
	}
	if projectPublicID == "" {
		projectPublicID = DefaultChatID
	}
	row := &VersionRow{ID: id}
	var diagramJSON []byte
	var dotContent sql.NullString
	err := r.db.QueryRow(`
		SELECT user_firebase_uid, project_public_id, version_number, title, yaml_content, diagram_json, dot_content, created_at, source
		FROM diagram_versions
		WHERE id = $1 AND user_firebase_uid = $2 AND project_public_id = $3
	`, id, userID, projectPublicID).Scan(&row.UserID, &row.ChatID, &row.VersionNumber, &row.Title,
		&row.YAMLContent, &diagramJSON, &dotContent, &row.CreatedAt, &row.Source)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	row.GraphJSON, row.DetectionsJSON = extractGraphAndDetectionsFromDiagramJSON(diagramJSON)
	if dotContent.Valid {
		row.DOTContent = dotContent.String
	}
	return row, nil
}

// ParseGraphAndDetections deserializes graph and detections from diagram_json (envelope) into the given pointers.
// For *domain.Graph, canvas_json-style payloads (nodes as JSON array) are normalized before decode.
func ParseGraphAndDetections(row *VersionRow, graphPtr interface{}, detectionsPtr interface{}) error {
	if len(row.GraphJSON) > 0 {
		if g, ok := graphPtr.(*domain.Graph); ok {
			if err := decodeGraphJSONFlexible(row.GraphJSON, g); err != nil {
				return err
			}
		} else {
			if err := json.Unmarshal(row.GraphJSON, graphPtr); err != nil {
				return err
			}
		}
	}
	if len(row.DetectionsJSON) > 0 {
		if err := json.Unmarshal(row.DetectionsJSON, detectionsPtr); err != nil {
			return err
		}
	}
	return nil
}
