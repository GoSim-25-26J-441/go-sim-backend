package http

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/auth/domain"
)

// GetProfile returns the current user's profile
func (h *Handler) GetProfile(c *gin.Context) {
	firebaseUID := c.GetString("firebase_uid")
	if firebaseUID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	user, err := h.authService.GetUserByFirebaseUID(firebaseUID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"user": user})
}

// SyncUser syncs Firebase user data to PostgreSQL
// This endpoint is called after Firebase authentication to ensure user exists in our DB
// Accepts optional JSON body with display_name, photo_url, organization, role, and preferences
func (h *Handler) SyncUser(c *gin.Context) {
	firebaseUID := c.GetString("firebase_uid")
	email := c.GetString("email")

	if firebaseUID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	// Parse optional request body for additional user data
	var body struct {
		Email        string                 `json:"email,omitempty"`
		DisplayName  *string                `json:"display_name,omitempty"`
		PhotoURL     *string                `json:"photo_url,omitempty"`
		Organization *string                `json:"organization,omitempty"`
		Role         string                 `json:"role,omitempty"`
		Preferences  map[string]interface{} `json:"preferences,omitempty"`
	}

	// Try to bind JSON body (ignore error if body is empty - this is optional)
	// But if body is provided and invalid, we should handle it
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body", "details": err.Error()})
			return
		}
	}

	// Email is required - prioritize: body > token > fallback
	if body.Email != "" {
		email = body.Email
	} else if email == "" {
		// If email is not in token or body, use a fallback based on firebase_uid
		email = firebaseUID + "@firebase.local" // Fallback email
	}

	// Sync user with data from token and request body
	req := &domain.CreateUserRequest{
		FirebaseUID:  firebaseUID,
		Email:        email,
		DisplayName:  body.DisplayName,
		PhotoURL:     body.PhotoURL,
		Organization: body.Organization,
		Role:         body.Role,
		Preferences:  body.Preferences,
	}

	user, err := h.authService.SyncUser(req)
	if err != nil {
		// Check for specific database errors
		if err.Error() == "pq: duplicate key value violates unique constraint \"users_email_key\"" {
			c.JSON(http.StatusConflict, gin.H{"error": "email already exists"})
			return
		}
		// Log the actual error for debugging (in production, use proper logging)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to sync user", "details": err.Error()})
		return
	}

	// Record login
	_ = h.authService.RecordLogin(firebaseUID)

	c.JSON(http.StatusOK, gin.H{"user": user})
}

// UpdateProfile updates the user's profile
func (h *Handler) UpdateProfile(c *gin.Context) {
	firebaseUID := c.GetString("firebase_uid")
	if firebaseUID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	var req struct {
		DisplayName  *string                `json:"display_name,omitempty"`
		PhotoURL     *string                `json:"photo_url,omitempty"`
		Organization *string                `json:"organization,omitempty"`
		Preferences  map[string]interface{} `json:"preferences,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	updateReq := &domain.UpdateUserRequest{
		DisplayName:  req.DisplayName,
		PhotoURL:     req.PhotoURL,
		Organization: req.Organization,
		Preferences:  req.Preferences,
	}

	user, err := h.authService.UpdateUser(firebaseUID, updateReq)
	if err != nil {
		if err == domain.ErrUserNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"user": user})
}
