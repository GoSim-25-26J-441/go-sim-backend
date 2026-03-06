package amg_apd_version

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	DefaultUserID = "TestUser123"
	DefaultChatID = "TestChat123"
)

// VersionRow represents one row in diagram_versions with source = 'amg_apd'.
type VersionRow struct {
	ID             string
	UserID         string
	ChatID         string
	VersionNumber  int
	Title          string
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

func NewRepo(db *sql.DB) *Repo {
	return &Repo{db: db}
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

	if title == "" {
		title = fmt.Sprintf("diagramV%d", nextVersion)
	}

	// Pack graph and detections into diagram_json as a single JSON object.
	var payload struct {
		Graph      json.RawMessage `json:"graph,omitempty"`
		Detections json.RawMessage `json:"detections,omitempty"`
	}
	if len(graphJSON) > 0 {
		payload.Graph = json.RawMessage(graphJSON)
	}
	if len(detectionsJSON) > 0 {
		payload.Detections = json.RawMessage(detectionsJSON)
	}
	diagramJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	id := uuid.New().String()
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
			dot_content
		)
		VALUES ($1, $2, $3, $4, 'amg_apd', $5, $6, $7, $8)
	`, id, userID, chatID, nextVersion, title, yamlContent, diagramJSON, dotContent)
	if err != nil {
		return nil, err
	}

	row := &VersionRow{
		ID:             id,
		UserID:         userID,
		ChatID:         chatID,
		VersionNumber:  nextVersion,
		Title:          title,
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
		WHERE user_firebase_uid = $1 AND project_public_id = $2 AND source = 'amg_apd'
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
		// Unpack graph and detections from diagram_json
		if len(diagramJSON) > 0 {
			var payload struct {
				Graph      json.RawMessage `json:"graph,omitempty"`
				Detections json.RawMessage `json:"detections,omitempty"`
			}
			if err := json.Unmarshal(diagramJSON, &payload); err == nil {
				if len(payload.Graph) > 0 {
					row.GraphJSON = payload.Graph
				}
				if len(payload.Detections) > 0 {
					row.DetectionsJSON = payload.Detections
				}
			}
		}
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
		SELECT user_firebase_uid, project_public_id, version_number, title, yaml_content, diagram_json, dot_content, created_at
		FROM diagram_versions
		WHERE id = $1 AND source = 'amg_apd'
	`, id).Scan(&row.UserID, &row.ChatID, &row.VersionNumber, &row.Title,
		&row.YAMLContent, &diagramJSON, &dotContent, &row.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(diagramJSON) > 0 {
		var payload struct {
			Graph      json.RawMessage `json:"graph,omitempty"`
			Detections json.RawMessage `json:"detections,omitempty"`
		}
		if err := json.Unmarshal(diagramJSON, &payload); err == nil {
			if len(payload.Graph) > 0 {
				row.GraphJSON = payload.Graph
			}
			if len(payload.Detections) > 0 {
				row.DetectionsJSON = payload.Detections
			}
		}
	}
	if dotContent.Valid {
		row.DOTContent = dotContent.String
	}
	return row, nil
}

// GetByIDForUserChat returns a version by id only if it belongs to the given user_id and chat_id.
func (r *Repo) GetByIDForUserChat(id, userID, chatID string) (*VersionRow, error) {
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
	if row.UserID != userID || row.ChatID != chatID {
		return nil, nil
	}
	return row, nil
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

// VersionSummary is a lightweight summary for list responses.
type VersionSummary struct {
	ID            string    `json:"id"`
	VersionNumber int       `json:"version_number"`
	Title         string    `json:"title"`
	CreatedAt     time.Time `json:"created_at"`
}

// ListSummariesByUserChat returns lightweight summaries for user/chat.
func (r *Repo) ListSummariesByUserChat(userID, chatID string) ([]VersionSummary, error) {
	if userID == "" {
		userID = DefaultUserID
	}
	if chatID == "" {
		chatID = DefaultChatID
	}
	rows, err := r.db.Query(`
		SELECT id, version_number, title, created_at
		FROM diagram_versions
		WHERE user_firebase_uid = $1 AND project_public_id = $2 AND source = 'amg_apd'
		ORDER BY version_number DESC
	`, userID, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []VersionSummary
	for rows.Next() {
		var s VersionSummary
		if err := rows.Scan(&s.ID, &s.VersionNumber, &s.Title, &s.CreatedAt); err != nil {
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
		SELECT id, user_firebase_uid, project_public_id, version_number, title, yaml_content, diagram_json, dot_content, created_at
		FROM diagram_versions
		WHERE user_firebase_uid = $1 AND project_public_id = $2 AND source = 'amg_apd'
		ORDER BY version_number DESC
		LIMIT 1
	`, userID, projectPublicID).Scan(
		&row.ID, &row.UserID, &row.ChatID, &row.VersionNumber, &row.Title,
		&row.YAMLContent, &diagramJSON, &dotContent, &row.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(diagramJSON) > 0 {
		var payload struct {
			Graph      json.RawMessage `json:"graph,omitempty"`
			Detections json.RawMessage `json:"detections,omitempty"`
		}
		if err := json.Unmarshal(diagramJSON, &payload); err == nil {
			if len(payload.Graph) > 0 {
				row.GraphJSON = payload.Graph
			}
			if len(payload.Detections) > 0 {
				row.DetectionsJSON = payload.Detections
			}
		}
	}
	if dotContent.Valid {
		row.DOTContent = dotContent.String
	}
	return row, nil
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

	var payload struct {
		Graph      json.RawMessage `json:"graph,omitempty"`
		Detections json.RawMessage `json:"detections,omitempty"`
	}
	if len(graphJSON) > 0 {
		payload.Graph = json.RawMessage(graphJSON)
	}
	if len(detectionsJSON) > 0 {
		payload.Detections = json.RawMessage(detectionsJSON)
	}
	diagramJSON, err := json.Marshal(payload)
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

// ParseGraphAndDetections deserializes graph and detections from diagram_json (envelope) into the given pointers.
func ParseGraphAndDetections(row *VersionRow, graphPtr interface{}, detectionsPtr interface{}) error {
	if len(row.GraphJSON) > 0 {
		if err := json.Unmarshal(row.GraphJSON, graphPtr); err != nil {
			return err
		}
	}
	if len(row.DetectionsJSON) > 0 {
		if err := json.Unmarshal(row.DetectionsJSON, detectionsPtr); err != nil {
			return err
		}
	}
	return nil
}
