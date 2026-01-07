package http

import "github.com/gin-gonic/gin"

// RegisterEngineCallbackRoutes registers routes intended to be called by the simulation engine (no Firebase auth).
func (h *Handler) RegisterEngineCallbackRoutes(rg *gin.RouterGroup) {
	// Support both: generic callback and run-specific callback
	rg.POST("/runs/callback", h.EngineRunCallback)           // Generic callback (legacy - uses run_id in body)
	rg.POST("/runs/callback/:run_id", h.EngineRunCallbackByID) // Run-specific callback (recommended - run_id in URL)
}


