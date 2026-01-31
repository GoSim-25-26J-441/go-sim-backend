package repository

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/auth/domain"
)

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

// GetByFirebaseUID retrieves a user by their Firebase UID
func (r *UserRepository) GetByFirebaseUID(uid string) (*domain.User, error) {
	query := `
		SELECT firebase_uid, email, display_name, photo_url, role, organization, 
		       preferences, created_at, updated_at, last_login_at
		FROM users
		WHERE firebase_uid = $1
	`

	var user domain.User
	var preferencesJSON []byte
	var displayName, photoURL, organization sql.NullString
	var lastLoginAt sql.NullTime

	err := r.db.QueryRow(query, uid).Scan(
		&user.FirebaseUID,
		&user.Email,
		&displayName,
		&photoURL,
		&user.Role,
		&organization,
		&preferencesJSON,
		&user.CreatedAt,
		&user.UpdatedAt,
		&lastLoginAt,
	)

	if err == sql.ErrNoRows {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}

	// Handle nullable fields
	if displayName.Valid {
		user.DisplayName = &displayName.String
	}
	if photoURL.Valid {
		user.PhotoURL = &photoURL.String
	}
	if organization.Valid {
		user.Organization = &organization.String
	}
	if lastLoginAt.Valid {
		user.LastLoginAt = &lastLoginAt.Time
	}

	// Parse JSONB preferences
	if len(preferencesJSON) > 0 {
		if err := json.Unmarshal(preferencesJSON, &user.Preferences); err != nil {
			user.Preferences = make(map[string]interface{})
		}
	} else {
		user.Preferences = make(map[string]interface{})
	}

	return &user, nil
}

// Create creates a new user
func (r *UserRepository) Create(user *domain.User) error {
	query := `
		INSERT INTO users (firebase_uid, email, display_name, photo_url, role, organization, preferences)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at, updated_at
	`

	preferencesJSON, err := json.Marshal(user.Preferences)
	if err != nil {
		preferencesJSON = []byte("{}")
	}

	err = r.db.QueryRow(
		query,
		user.FirebaseUID,
		user.Email,
		user.DisplayName,
		user.PhotoURL,
		user.Role,
		user.Organization,
		preferencesJSON,
	).Scan(&user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		return err
	}

	return nil
}

// Update updates user information
func (r *UserRepository) Update(user *domain.User) error {
	query := `
		UPDATE users
		SET display_name = $2, photo_url = $3, organization = $4, preferences = $5, updated_at = NOW()
		WHERE firebase_uid = $1
		RETURNING updated_at
	`

	preferencesJSON, err := json.Marshal(user.Preferences)
	if err != nil {
		preferencesJSON = []byte("{}")
	}

	err = r.db.QueryRow(
		query,
		user.FirebaseUID,
		user.DisplayName,
		user.PhotoURL,
		user.Organization,
		preferencesJSON,
	).Scan(&user.UpdatedAt)

	if err == sql.ErrNoRows {
		return domain.ErrUserNotFound
	}
	if err != nil {
		return err
	}

	return nil
}

// UpdateLastLogin updates the last login timestamp
func (r *UserRepository) UpdateLastLogin(uid string) error {
	query := `
		UPDATE users
		SET last_login_at = NOW()
		WHERE firebase_uid = $1
	`

	result, err := r.db.Exec(query, uid)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return domain.ErrUserNotFound
	}

	return nil
}

// Upsert creates or updates a user (useful for syncing from Firebase)
func (r *UserRepository) Upsert(user *domain.User) error {
	query := `
		INSERT INTO users (firebase_uid, email, display_name, photo_url, role, organization, preferences)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (firebase_uid) DO UPDATE
		SET email = EXCLUDED.email,
		    display_name = EXCLUDED.display_name,
		    photo_url = EXCLUDED.photo_url,
		    updated_at = NOW()
		RETURNING created_at, updated_at
	`

	preferencesJSON, err := json.Marshal(user.Preferences)
	if err != nil {
		preferencesJSON = []byte("{}")
	}

	var createdAt, updatedAt time.Time
	err = r.db.QueryRow(
		query,
		user.FirebaseUID,
		user.Email,
		user.DisplayName,
		user.PhotoURL,
		user.Role,
		user.Organization,
		preferencesJSON,
	).Scan(&createdAt, &updatedAt)

	if err != nil {
		return err
	}

	user.CreatedAt = createdAt
	user.UpdatedAt = updatedAt

	return nil
}

