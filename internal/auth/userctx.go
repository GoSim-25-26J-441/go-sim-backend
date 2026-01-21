package auth

import (
	"net/http"
	"strings"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/users"
	"github.com/gin-gonic/gin"
)

const (
	CtxFirebaseUID = "firebase_uid"
)

func WithUser(userRepo *users.Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		fuid := strings.TrimSpace(c.GetHeader("X-User-Id"))
		if fuid == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"ok":    false,
				"error": "missing X-User-Id",
			})
			c.Abort()
			return
		}

		_, err := userRepo.EnsureUser(c.Request.Context(), users.UpsertUser{
			FirebaseUID: fuid,
			Email:       strings.TrimSpace(c.GetHeader("X-User-Email")),
			DisplayName: strings.TrimSpace(c.GetHeader("X-User-Name")),
			PhotoURL:    strings.TrimSpace(c.GetHeader("X-User-Photo")),
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"ok":    false,
				"error": "ensure user: " + err.Error(),
			})
			c.Abort()
			return
		}

		c.Set(CtxFirebaseUID, fuid)
		c.Next()
	}
}

func UserFirebaseUID(c *gin.Context) string {
	return strings.TrimSpace(c.GetString(CtxFirebaseUID))
}

func UserDBID(c *gin.Context) string {
	return UserFirebaseUID(c)
}
