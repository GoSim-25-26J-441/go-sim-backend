package http

import "github.com/gin-gonic/gin"

// Register attaches project routes to the given router group.
func (h *Handler) Register(rg *gin.RouterGroup) {
	// Project CRUD routes
	rg.POST("", h.create)
	rg.GET("", h.list)
	rg.PATCH("/:public_id", h.rename)
	rg.DELETE("/:public_id", h.delete)

	// Chat routes - specific route must come before parameterized routes
	rg.GET("/chats", h.listAllThreads) // List all threads for user (no project ID required)
	// Project-specific chat routes
	rg.POST("/:public_id/chats", h.createThread)
	rg.GET("/:public_id/chats", h.listThreads)
	rg.PATCH("/:public_id/chats/:thread_id", h.updateThreadBinding)
	rg.POST("/:public_id/chats/:thread_id/messages", h.postMessage)
	rg.GET("/:public_id/chats/:thread_id/messages", h.listMessages)

	// Diagram routes
	rg.POST("/:public_id/diagram", h.createVersion)
	rg.POST("/:public_id/diagram/image", h.uploadDiagramImage)
	rg.GET("/:public_id/diagram/latest", h.latest)
	rg.GET("/:public_id/diagram/images", h.listDiagramImages)
	rg.PATCH("/:public_id/diagram/:version_id/title", h.updateDiagramTitle)
	rg.PATCH("/:public_id/diagram/:version_id", h.updateDiagramVersion)

	// Summary route
	rg.GET("/:public_id/summary", h.summary)
}
