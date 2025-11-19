package http

import "github.com/gin-gonic/gin"

func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.GET("/health", h.health)
	rg.POST("/ingest", h.ingest)

	jobs := rg.Group("/jobs/:id")
	jobs.GET("/intermediate", h.intermediate)
	jobs.POST("/fuse", h.fuse)
	jobs.GET("/export", h.export)
	jobs.GET("/report", h.report)
	jobs.POST("/chat", h.chat)
	rg.GET("/rag/search", h.ragSearch)
	rg.GET("/jobs/:id/chat/history", h.chatHistory)
	rg.DELETE("/jobs/:id/chat/history", h.chatClear)
	rg.GET("/jobs/:id/chat/stream", h.chatStream)

}
