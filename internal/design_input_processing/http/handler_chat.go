package http

import (
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/http/handlers"
	"github.com/gin-gonic/gin"
)

func (h *Handler) chat(c *gin.Context) {
	handlers.Chat(c, h.UpstreamURL, h.OllamaURL)
}
