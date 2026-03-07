package http

import "github.com/gin-gonic/gin"

// Register registers the simulation routes
func (h *Handler) Register(rg *gin.RouterGroup) {
	// Project-scoped routes (project_id in path)
	rg.POST("/projects/:project_id/runs", h.CreateRunForProject)
	rg.GET("/projects/:project_id/runs", h.ListRunsForProject)

	// User-level routes
	rg.POST("/runs", h.CreateRun)
	rg.GET("/runs", h.ListRuns)
	rg.GET("/runs/:id", h.GetRun)
	rg.GET("/runs/:id/candidates", h.GetRunCandidates)
	rg.GET("/runs/:id/metrics", h.GetRunMetrics)
	rg.GET("/runs/:id/events", h.StreamRunEvents) // SSE endpoint for real-time updates
	rg.GET("/runs/engine/:engine_run_id", h.GetRunByEngineID)
	rg.PUT("/runs/:id", h.UpdateRun)
	rg.PATCH("/runs/:id/configuration", h.UpdateConfiguration) // Dynamic configuration update (services, workload, policies)
	rg.PATCH("/runs/:id/workload", h.UpdateWorkload)           // Dynamic workload rate update per BACKEND_INTEGRATION.md
	rg.DELETE("/runs/:id", h.DeleteRun)
}
