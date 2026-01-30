package http

import "github.com/gin-gonic/gin"

// Register attaches project routes to the given router group.
func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.POST("", h.create)
	rg.GET("", h.list)
	rg.PATCH("/:public_id", h.rename)
	rg.DELETE("/:public_id", h.delete)
}

