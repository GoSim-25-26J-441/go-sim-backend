package handlers

import "github.com/gin-gonic/gin"

func Health(c *gin.Context, upstreamURL string) {
	c.JSON(200, gin.H{
		"ok":        true,
		"component": "design-input-processing",
		"upstream":  upstreamURL,
	})
}
