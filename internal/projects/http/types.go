package http

import "github.com/GoSim-25-26J-441/go-sim-backend/internal/projects/repository"

// Handler bundles the dependencies for projects HTTP endpoints.
type Handler struct {
	repo *repository.Repo
}

func New(repo *repository.Repo) *Handler {
	return &Handler{repo: repo}
}

