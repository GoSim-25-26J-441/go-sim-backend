package http

import (
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/_legacy_http/handlers"
	"github.com/gin-gonic/gin"
)

func (h *Handler) chat(c *gin.Context) {
	handlers.Chat(c, h.UpstreamURL, h.OllamaURL)
}

func (h *Handler) chatStream(c *gin.Context) {
	handlers.ChatStream(c, h.UpstreamURL, h.OllamaURL)
}

func (h *Handler) graphviz(c *gin.Context) {
	handlers.Graphviz(c, h.UpstreamURL)
}
