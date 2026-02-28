package http

import "github.com/gin-gonic/gin"

func (h *Handler) health(c *gin.Context) {
	c.JSON(200, gin.H{
		"ok":        true,
		"component": "design-input-processing",
		"upstream":  h.upstreamURL,
	})
}
