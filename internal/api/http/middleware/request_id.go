package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// requestIDKey is the key used to store request ID in context
type requestIDKey struct{}

// RequestIDMiddleware ensures every request has a stable request ID.
// - Reads X-Request-Id header if present
// - Otherwise generates a new one
// - Stores it in both Gin context and standard context as "request_id"
// - Echoes it back in response header X-Request-Id
// - Logs request details (method, path, status, latency)
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader("X-Request-Id")
		if strings.TrimSpace(rid) == "" {
			rid = newRequestID()
		}

		// Store in Gin context (for handlers that use c.GetString)
		c.Set("request_id", rid)
		
		// Store in standard context (for services that use context.Context)
		ctx := context.WithValue(c.Request.Context(), requestIDKey{}, rid)
		c.Request = c.Request.WithContext(ctx)
		
		// Set response header
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

// GetRequestID extracts the request ID from a standard context
func GetRequestID(ctx context.Context) string {
	if rid, ok := ctx.Value(requestIDKey{}).(string); ok {
		return rid
	}
	return ""
}

func newRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err == nil {
		return hex.EncodeToString(b)
	}
	// fallback (should be rare)
	return time.Now().Format("20060102T150405.000000000")
}
