package rag

import "github.com/gin-gonic/gin"

// Register mounts RAG routes on the given group.
// Call with api.Group("/design-input") so routes are at /api/v1/design-input/rag.
func Register(rg *gin.RouterGroup, h *Handler) {
	rag := rg.Group("/rag")
	{
		rag.GET("", h.Search)
		rag.GET("/search", h.Search)
		rag.POST("", h.Ingest)
		rag.GET("/requirements-questions", h.GetRequirementsQuestions)
	}
}
