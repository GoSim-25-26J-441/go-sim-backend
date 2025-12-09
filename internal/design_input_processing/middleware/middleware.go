package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// RequestIDMiddleware ensures every request has a stable request ID.
// - Reads X-Request-Id header if present
// - Otherwise generates a new one
// - Stores it in context as "request_id"
// - Echoes it back in response header X-Request-Id
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader("X-Request-Id")
		if strings.TrimSpace(rid) == "" {
			rid = newRequestID()
		}

		c.Set("request_id", rid)
		c.Writer.Header().Set("X-Request-Id", rid)

		start := time.Now()
		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		log.Printf(
			"[req] id=%s method=%s path=%s status=%d latency=%s",
			rid,
			c.Request.Method,
			c.Request.URL.Path,
			status,
			latency,
		)
	}
}

func newRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err == nil {
		return hex.EncodeToString(b)
	}
	// fallback (should be rare)
	return time.Now().Format("20060102T150405.000000000")
}
