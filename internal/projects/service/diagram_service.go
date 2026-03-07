package service

import (
	"context"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/projects/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/projects/repository"
)

// DiagramService handles diagram-related business logic
type DiagramService struct {
	repo *repository.DiagramRepository
}

// NewDiagramService creates a new diagram service
func NewDiagramService(repo *repository.DiagramRepository) *DiagramService {
	return &DiagramService{
		repo: repo,
	}
}

// CreateVersion creates a new diagram version
func (s *DiagramService) CreateVersion(ctx context.Context, userID, publicID string, input domain.CreateVersionInput) (*domain.DiagramVersion, error) {
	return s.repo.CreateVersion(ctx, userID, publicID, input)
}

// GetLatest retrieves the latest diagram version for a project
func (s *DiagramService) GetLatest(ctx context.Context, userID, publicID string) (*domain.DiagramVersion, error) {
	return s.repo.Latest(ctx, userID, publicID)
}

// ListAllVersions returns all diagram versions for a project
func (s *DiagramService) ListAllVersions(ctx context.Context, userID, publicID string) ([]domain.DiagramVersion, error) {
	return s.repo.ListAllVersions(ctx, userID, publicID)
}

// UpdateTitle updates the title of a diagram version for a given user and project.
func (s *DiagramService) UpdateTitle(ctx context.Context, userID, publicID, versionID, title string) (bool, error) {
	return s.repo.UpdateTitle(ctx, userID, publicID, versionID, title)
}
