package http

import "github.com/gin-gonic/gin"

func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.POST("/:public_id/chats", h.createThread)
	rg.GET("/:public_id/chats", h.listThreads)

	rg.POST("/:public_id/chats/:thread_id/messages", h.postMessage)
	rg.GET("/:public_id/chats/:thread_id/messages", h.listMessages)
}
