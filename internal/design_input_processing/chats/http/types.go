package http

import (
	dipllm "github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/llm"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/chats/repository"
)

type Handler struct {
	repo *repository.Repo
	uigp *dipllm.UIGPClient
}

func New(repo *repository.Repo, uigp *dipllm.UIGPClient) *Handler {
	return &Handler{repo: repo, uigp: uigp}
}
