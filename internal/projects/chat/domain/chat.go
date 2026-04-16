package domain

import (
	"errors"
	"time"
)

// ErrDiagramPayloadNotReady is returned when the project/thread points at a diagram
// version but spec_summary and diagram_json are still empty in the database (often a
// race: chat message sent before the diagram body is persisted).
var ErrDiagramPayloadNotReady = errors.New("diagram content not ready for this version; save the diagram and retry")

const (
	BindingFollowLatest = "FOLLOW_LATEST"
	BindingPinned       = "PINNED"
)

type Thread struct {
	ID                     string    `json:"id"`
	ProjectID              string    `json:"project_id,omitempty"`
	ProjectPublicID        string    `json:"project_public_id,omitempty"`
	Title                  *string   `json:"title,omitempty"`
	BindingMode            string    `json:"binding_mode"`
	PinnedDiagramVersionID *string   `json:"pinned_diagram_version_id,omitempty"`
	CreatedAt              time.Time `json:"created_at"`
}

type Attachment struct {
	ID            string    `json:"id"`
	Kind          string    `json:"kind"`
	ObjectKey     string    `json:"object_key"`
	MimeType      *string   `json:"mime_type,omitempty"`
	FileName      *string   `json:"file_name,omitempty"`
	FileSizeBytes *int64    `json:"file_size_bytes,omitempty"`
	Width         *int      `json:"width,omitempty"`
	Height        *int      `json:"height,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

type Message struct {
	ID                   string       `json:"id"`
	ThreadID             string        `json:"thread_id"`
	ProjectID            string        `json:"project_id,omitempty"`
	Role                 string        `json:"role"`
	Content              string        `json:"content"`
	Source               *string       `json:"source,omitempty"`
	Refs                 []string      `json:"refs,omitempty"`
	DiagramVersionIDUsed *string       `json:"diagram_version_id_used,omitempty"`
	CreatedAt            time.Time     `json:"created_at"`
	Attachments          []Attachment  `json:"attachments,omitempty"`
}
