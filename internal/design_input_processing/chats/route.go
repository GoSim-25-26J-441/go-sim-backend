package chats

import "github.com/gin-gonic/gin"

func RegisterProjectChatRoutes(projectsGroup *gin.RouterGroup, h *Handler) {
	projectsGroup.POST("/:public_id/chats", h.createThread)
	projectsGroup.GET("/:public_id/chats", h.listThreads)

	projectsGroup.POST("/:public_id/chats/:thread_id/messages", h.postMessage)
	projectsGroup.GET("/:public_id/chats/:thread_id/messages", h.listMessages)
}
