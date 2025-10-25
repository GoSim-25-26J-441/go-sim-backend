package http

import (
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/http/handlers"
	"github.com/gin-gonic/gin"
)

func (h *Handler) intermediate(c *gin.Context) { handlers.Intermediate(c, h.UpstreamURL) }
func (h *Handler) fuse(c *gin.Context)         { handlers.Fuse(c, h.UpstreamURL) }
func (h *Handler) export(c *gin.Context)       { handlers.Export(c, h.UpstreamURL) }
func (h *Handler) report(c *gin.Context)       { handlers.Report(c, h.UpstreamURL) }
