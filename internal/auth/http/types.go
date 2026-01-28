package http

import "github.com/GoSim-25-26J-441/go-sim-backend/internal/auth/service"

type Handler struct {
	authService *service.AuthService
}

func New(authService *service.AuthService) *Handler {
	return &Handler{
		authService: authService,
	}
}
