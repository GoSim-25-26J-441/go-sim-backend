package amg_apd_version

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	DefaultUserID = "TestUser123"
	DefaultChatID = "TestChat123"
)

// VersionRow represents one row in amg_apd_versions.
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
		FROM amg_apd_versions
		WHERE user_id = $1 AND chat_id = $2
	`, userID, chatID).Scan(&nextVersion)
	if err != nil {
		return nil, err
	}

	id := uuid.New().String()
	_, err = r.db.Exec(`
		INSERT INTO amg_apd_versions (id, user_id, chat_id, version_number, title, yaml_content, graph_json, dot_content, detections_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, id, userID, chatID, nextVersion, title, yamlContent, graphJSON, dotContent, detectionsJSON)
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
		SELECT id, user_id, chat_id, version_number, title, yaml_content, graph_json, dot_content, detections_json, created_at
		FROM amg_apd_versions
		WHERE user_id = $1 AND chat_id = $2
		ORDER BY version_number DESC
	`, userID, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []VersionRow
	for rows.Next() {
		var row VersionRow
		var graphJSON, detectionsJSON []byte
		var dotContent sql.NullString
		err := rows.Scan(&row.ID, &row.UserID, &row.ChatID, &row.VersionNumber, &row.Title,
			&row.YAMLContent, &graphJSON, &dotContent, &detectionsJSON, &row.CreatedAt)
		if err != nil {
			return nil, err
		}
		row.GraphJSON = graphJSON
		row.DetectionsJSON = detectionsJSON
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
	var graphJSON, detectionsJSON []byte
	var dotContent sql.NullString
	err := r.db.QueryRow(`
		SELECT user_id, chat_id, version_number, title, yaml_content, graph_json, dot_content, detections_json, created_at
		FROM amg_apd_versions
		WHERE id = $1
	`, id).Scan(&row.UserID, &row.ChatID, &row.VersionNumber, &row.Title,
		&row.YAMLContent, &graphJSON, &dotContent, &detectionsJSON, &row.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	row.GraphJSON = graphJSON
	row.DetectionsJSON = detectionsJSON
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
	res, err := r.db.Exec(`DELETE FROM amg_apd_versions WHERE id = $1`, id)
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
	res, err := r.db.Exec(`DELETE FROM amg_apd_versions WHERE id = $1 AND user_id = $2 AND chat_id = $3`, id, userID, chatID)
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
		FROM amg_apd_versions
		WHERE user_id = $1 AND chat_id = $2
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

// ParseGraphAndDetections deserializes graph_json and detections_json from a row into the given pointers.
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
