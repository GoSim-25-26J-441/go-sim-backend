package http

import (
	handlers "github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/http/handlers"

	"github.com/gin-gonic/gin"
)

func (h *Handler) chatHistory(c *gin.Context) {
	handlers.ChatHistory(c)
}

func (h *Handler) chatClear(c *gin.Context) {
	handlers.ChatClear(c)
}
