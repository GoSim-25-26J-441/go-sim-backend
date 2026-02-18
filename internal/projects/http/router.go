package http

import "github.com/gin-gonic/gin"

// Register attaches project routes to the given router group.
func (h *Handler) Register(rg *gin.RouterGroup) {
	// Project CRUD routes
	rg.POST("", h.create)
	rg.GET("", h.list)
	rg.PATCH("/:public_id", h.rename)
	rg.DELETE("/:public_id", h.delete)

	// Chat routes
	rg.POST("/:public_id/chats", h.createThread)
	rg.GET("/:public_id/chats", h.listThreads)
	rg.POST("/:public_id/chats/:thread_id/messages", h.postMessage)
	rg.GET("/:public_id/chats/:thread_id/messages", h.listMessages)

	// Diagram routes
	rg.POST("/:public_id/diagram", h.createVersion)
	rg.GET("/:public_id/diagram/latest", h.latest)
}
