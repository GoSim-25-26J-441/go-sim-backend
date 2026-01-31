package middleware

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

func APIKeyMiddleware() gin.HandlerFunc {
	expected := os.Getenv("API_KEY")

	return func(c *gin.Context) {
		if expected == "" {
			expected = "super-secret-key-123"
		}

		key := c.GetHeader("X-API-Key")

		if key == "" || key != expected {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"ok":    false,
				"error": "invalid API key",
			})
			return
		}

		c.Next()
	}
}
