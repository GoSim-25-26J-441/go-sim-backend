package http

import (
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/projects/service"
)

// Handler bundles the dependencies for projects HTTP endpoints.
type Handler struct {
	projectService *service.ProjectService
	chatService    *service.ChatService
	diagramService *service.DiagramService
}

// New creates a new projects HTTP handler with the given services.
func New(projectService *service.ProjectService, chatService *service.ChatService, diagramService *service.DiagramService) *Handler {
	return &Handler{
		projectService: projectService,
		chatService:    chatService,
		diagramService: diagramService,
	}
}

