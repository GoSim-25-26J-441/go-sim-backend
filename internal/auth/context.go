package auth

import (
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	CtxFirebaseUID = "firebase_uid"
)

// UserFirebaseUID extracts the Firebase UID from the Gin context
// This is set by FirebaseAuthMiddleware
func UserFirebaseUID(c *gin.Context) string {
	return strings.TrimSpace(c.GetString(CtxFirebaseUID))
}
