package http

import "github.com/gin-gonic/gin"

func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.GET("/profile", h.GetProfile)
	rg.POST("/sync", h.SyncUser)
	rg.PUT("/profile", h.UpdateProfile)
}

