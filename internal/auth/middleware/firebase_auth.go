package middleware

import (
	"context"
	"net/http"
	"strings"

	"firebase.google.com/go/v4/auth"
	"github.com/gin-gonic/gin"
)

// FirebaseAuthMiddleware validates Firebase ID tokens and extracts user info
func FirebaseAuthMiddleware(authClient *auth.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractToken(c)
		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization token"})
			c.Abort()
			return
		}

		decodedToken, err := authClient.VerifyIDToken(context.Background(), token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		// Store user info in context
		c.Set("firebase_uid", decodedToken.UID)

		// Extract email from claims if available
		if email, ok := decodedToken.Claims["email"].(string); ok {
			c.Set("email", email)
		}

		// Store the full token for access to other claims if needed
		c.Set("firebase_token", decodedToken)

		c.Next()
	}
}

// extractToken extracts the Bearer token from the Authorization header
func extractToken(c *gin.Context) string {
	bearerToken := c.GetHeader("Authorization")
	if len(bearerToken) > 7 && strings.HasPrefix(bearerToken, "Bearer ") {
		return bearerToken[7:]
	}
	return ""
}
