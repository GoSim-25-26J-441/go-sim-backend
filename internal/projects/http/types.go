package http

import (
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/projects/service"
	chatservice "github.com/GoSim-25-26J-441/go-sim-backend/internal/projects/chat/service"
	s3storage "github.com/GoSim-25-26J-441/go-sim-backend/internal/storage/s3"
)

// Handler bundles the dependencies for projects HTTP endpoints.
type Handler struct {
	projectService *service.ProjectService
	chatService    *chatservice.ChatService
	diagramService *service.DiagramService
	s3Client       *s3storage.Client
}

// New creates a new projects HTTP handler with the given services.
func New(projectService *service.ProjectService, chatService *chatservice.ChatService, diagramService *service.DiagramService, s3Client *s3storage.Client) *Handler {
	return &Handler{
		projectService: projectService,
		chatService:    chatService,
		diagramService: diagramService,
		s3Client:       s3Client,
	}
}

