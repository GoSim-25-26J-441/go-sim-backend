package domain

import "time"

// Project represents a single architecture project owned by a user.
// It is intentionally storage-agnostic and used across repository and HTTP layers.
type Project struct {
	PublicID  string    `json:"public_id"`
	Name      string    `json:"name"`
	Temporary bool      `json:"is_temporary"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

