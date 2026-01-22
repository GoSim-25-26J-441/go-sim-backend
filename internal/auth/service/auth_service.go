package service

import (
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/auth/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/auth/repository"
)

type AuthService struct {
	userRepo *repository.UserRepository
}

func NewAuthService(userRepo *repository.UserRepository) *AuthService {
	return &AuthService{
		userRepo: userRepo,
	}
}

// GetUserByFirebaseUID retrieves a user by Firebase UID
func (s *AuthService) GetUserByFirebaseUID(uid string) (*domain.User, error) {
	return s.userRepo.GetByFirebaseUID(uid)
}

// SyncUser creates or updates a user from Firebase Auth data
func (s *AuthService) SyncUser(req *domain.CreateUserRequest) (*domain.User, error) {
	// Try to get existing user first
	existingUser, err := s.userRepo.GetByFirebaseUID(req.FirebaseUID)
	
	if err == nil && existingUser != nil {
		// User exists, update with new data if provided
		// Preserve existing data if not provided in request
		if req.DisplayName != nil {
			existingUser.DisplayName = req.DisplayName
		}
		if req.PhotoURL != nil {
			existingUser.PhotoURL = req.PhotoURL
		}
		// Only update organization/role/preferences if they are provided (not empty)
		// This allows updates on subsequent logins without overwriting
		if req.Organization != nil {
			existingUser.Organization = req.Organization
		}
		if req.Role != "" {
			existingUser.Role = req.Role
		}
		// Merge preferences if provided (don't overwrite existing ones)
		if req.Preferences != nil && len(req.Preferences) > 0 {
			if existingUser.Preferences == nil {
				existingUser.Preferences = make(map[string]interface{})
			}
			// Merge new preferences with existing ones
			for k, v := range req.Preferences {
				existingUser.Preferences[k] = v
			}
		}
		
		if err := s.userRepo.Update(existingUser); err != nil {
			return nil, err
		}
		return existingUser, nil
	}

	// User doesn't exist, create new one
	user := &domain.User{
		FirebaseUID:  req.FirebaseUID,
		Email:        req.Email,
		DisplayName:  req.DisplayName,
		PhotoURL:     req.PhotoURL,
		Organization: req.Organization,
		Role:         req.Role,
		Preferences:  make(map[string]interface{}),
	}

	if user.Role == "" {
		user.Role = "user" // Default role
	}

	// Set preferences if provided
	if req.Preferences != nil {
		user.Preferences = req.Preferences
	}

	if err := s.userRepo.Create(user); err != nil {
		return nil, err
	}

	return user, nil
}

// UpdateUser updates user information
func (s *AuthService) UpdateUser(uid string, req *domain.UpdateUserRequest) (*domain.User, error) {
	user, err := s.userRepo.GetByFirebaseUID(uid)
	if err != nil {
		return nil, err
	}

	// Update fields if provided
	if req.DisplayName != nil {
		user.DisplayName = req.DisplayName
	}
	if req.PhotoURL != nil {
		user.PhotoURL = req.PhotoURL
	}
	if req.Organization != nil {
		user.Organization = req.Organization
	}
	
	// Merge preferences if provided (don't overwrite existing ones)
	if req.Preferences != nil && len(req.Preferences) > 0 {
		if user.Preferences == nil {
			user.Preferences = make(map[string]interface{})
		}
		// Merge new preferences with existing ones
		for k, v := range req.Preferences {
			user.Preferences[k] = v
		}
	}

	if err := s.userRepo.Update(user); err != nil {
		return nil, err
	}

	return user, nil
}

// RecordLogin updates the last login timestamp
func (s *AuthService) RecordLogin(uid string) error {
	return s.userRepo.UpdateLastLogin(uid)
}
