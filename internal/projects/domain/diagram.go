package domain

import (
	"encoding/json"
	"time"
)

type DiagramVersion struct {
	ID              string          `json:"id"`
	ProjectPublicID string          `json:"project_public_id"`
	UserFirebaseUID string          `json:"user_firebase_uid"`
	VersionNumber   int             `json:"version_number"`
	Title           string          `json:"title"`
	Source          string          `json:"source"`
	Hash            string          `json:"hash,omitempty"`
	ImageObjectKey  string          `json:"image_object_key,omitempty"`
	DiagramJSON     json.RawMessage `json:"diagram_json,omitempty"`
	SpecSummary     json.RawMessage `json:"spec_summary,omitempty"`
	YAMLContent     string          `json:"yaml_content,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
}

type CreateVersionInput struct {
	Source         string
	Title          string
	DiagramJSON    json.RawMessage
	ImageObjectKey string
	SpecSummary    json.RawMessage
	Hash           string
}

// UpdateVersionInPlaceInput updates an existing diagram_versions row in place (same id / version_number).
// DiagramJSON is required. SpecSummary: if empty, spec is regenerated from diagram_json (same as create).
// ImageObjectKey, Hash, Source: nil means leave unchanged; pointer to "" clears the column where applicable.
type UpdateVersionInPlaceInput struct {
	DiagramJSON    json.RawMessage
	SpecSummary    json.RawMessage
	ImageObjectKey *string
	Hash           *string
	Source         *string
}
