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
	Source          string          `json:"source"`
	Hash            string          `json:"hash,omitempty"`
	ImageObjectKey  string          `json:"image_object_key,omitempty"`
	DiagramJSON     json.RawMessage `json:"diagram_json,omitempty"`
	SpecSummary     json.RawMessage `json:"spec_summary,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
}

type CreateVersionInput struct {
	Source         string
	DiagramJSON    json.RawMessage
	ImageObjectKey string
	SpecSummary    json.RawMessage
	Hash           string
}
