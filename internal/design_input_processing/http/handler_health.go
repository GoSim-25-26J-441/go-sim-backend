package http

import (
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/http/handlers"
	"github.com/gin-gonic/gin"
)

func (h *Handler) health(c *gin.Context) {
	handlers.Health(c, h.UpstreamURL)
}
