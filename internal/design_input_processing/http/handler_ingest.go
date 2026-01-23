package http

import (
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/_legacy_http/handlers"
	"github.com/gin-gonic/gin"
)

func (h *Handler) ingest(c *gin.Context) {
	handlers.Ingest(c, h.UpstreamURL)
}
