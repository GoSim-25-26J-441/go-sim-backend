package http

import "github.com/gin-gonic/gin"

// Register registers the simulation routes
func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.POST("/runs", h.CreateRun)
	rg.GET("/runs", h.ListRuns)
	rg.GET("/runs/:id", h.GetRun)
	rg.GET("/runs/engine/:engine_run_id", h.GetRunByEngineID)
	rg.PUT("/runs/:id", h.UpdateRun)
	rg.DELETE("/runs/:id", h.DeleteRun)
}
