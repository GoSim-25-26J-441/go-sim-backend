package domain

import "time"

// User represents a user in the application
// Firebase UID is the primary identifier
type User struct {
	FirebaseUID  string                 `json:"firebase_uid" db:"firebase_uid"`
	Email        string                 `json:"email" db:"email"`
	DisplayName  *string                `json:"display_name,omitempty" db:"display_name"`
	PhotoURL     *string                `json:"photo_url,omitempty" db:"photo_url"`
	Role         string                 `json:"role" db:"role"`
	Organization *string                `json:"organization,omitempty" db:"organization"`
	Preferences  map[string]interface{} `json:"preferences,omitempty" db:"preferences"`
	CreatedAt    time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at" db:"updated_at"`
	LastLoginAt  *time.Time             `json:"last_login_at,omitempty" db:"last_login_at"`
}

// CreateUserRequest represents data needed to create a new user
type CreateUserRequest struct {
	FirebaseUID  string
	Email        string
	DisplayName  *string
	PhotoURL     *string
	Role         string
	Organization *string
	Preferences  map[string]interface{}
}

// UpdateUserRequest represents data for updating a user
type UpdateUserRequest struct {
	DisplayName  *string
	PhotoURL     *string
	Organization *string
	Preferences  map[string]interface{}
}
