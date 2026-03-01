package http

import "github.com/gin-gonic/gin"

func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.GET("/health", h.health)
	rg.POST("/ingest", h.ingest)

	jobs := rg.Group("/jobs")
	jobs.GET("", h.listJobsForUser)
	jobs.GET("/summary", h.listJobsSummary)

	jobGroup := rg.Group("/jobs/:id")
	jobGroup.GET("/intermediate", h.intermediate)
	jobGroup.POST("/fuse", h.fuse)
	jobGroup.GET("/export", h.export)
	jobGroup.GET("/report", h.report)
	jobGroup.GET("/graphviz", h.graphviz)

	rg.GET("/rag/search", h.ragSearch)
	rg.POST("/rag/reload", h.ragReload)
}
