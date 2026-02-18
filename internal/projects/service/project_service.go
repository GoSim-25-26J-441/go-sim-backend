package service

import (
	"context"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/projects/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/projects/repository"
)

// ProjectService handles project-related business logic
type ProjectService struct {
	repo *repository.ProjectRepository
}

// NewProjectService creates a new project service
func NewProjectService(repo *repository.ProjectRepository) *ProjectService {
	return &ProjectService{
		repo: repo,
	}
}

// Create creates a new project
func (s *ProjectService) Create(ctx context.Context, userID, name string, temporary bool) (*domain.Project, error) {
	return s.repo.Create(ctx, userID, name, temporary)
}

// List returns all projects for a user
func (s *ProjectService) List(ctx context.Context, userID string) ([]domain.Project, error) {
	return s.repo.List(ctx, userID)
}

// Rename updates a project's name
func (s *ProjectService) Rename(ctx context.Context, userID, publicID, newName string) (*domain.Project, error) {
	return s.repo.Rename(ctx, userID, publicID, newName)
}

// Delete soft-deletes a project
func (s *ProjectService) Delete(ctx context.Context, userID, publicID string) (bool, error) {
	return s.repo.SoftDelete(ctx, userID, publicID)
}
