package http

import "github.com/gin-gonic/gin"

// RegisterEngineCallbackRoutes registers routes intended to be called by the simulation engine (no Firebase auth).
func (h *Handler) RegisterEngineCallbackRoutes(rg *gin.RouterGroup) {
	rg.POST("/runs/callback", h.EngineRunCallback)
}


