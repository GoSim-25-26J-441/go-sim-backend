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
