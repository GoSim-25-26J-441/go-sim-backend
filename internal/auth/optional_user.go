package auth

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// OptionalUser sets a firebase uid in context without enforcing auth.
// - If X-User-Id is missing, it falls back to "demo-user".
// - Use this ONLY for development/testing.
func OptionalUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := strings.TrimSpace(c.GetHeader("X-User-Id"))
		if uid == "" {
			uid = "demo-user"
		}

		// store a "firebase uid" style value in context
		c.Set("user_id", uid)

		c.Next()
	}
}
