package http

import "github.com/gin-gonic/gin"

func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.POST("/:public_id/diagram", h.createVersion)
	rg.GET("/:public_id/diagram/latest", h.latest)
}
