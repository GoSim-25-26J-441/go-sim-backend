package auth

import (
	"net/http"
	"strings"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/auth/domain"
	authrepo "github.com/GoSim-25-26J-441/go-sim-backend/internal/auth/repository"
	"github.com/gin-gonic/gin"
)

const (
	CtxFirebaseUID = "firebase_uid"
)

func WithUser(userRepo *authrepo.UserRepository) gin.HandlerFunc {
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

		email := strings.TrimSpace(c.GetHeader("X-User-Email"))
		if email == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"ok":    false,
				"error": "missing X-User-Email",
			})
			c.Abort()
			return
		}

		displayName := strings.TrimSpace(c.GetHeader("X-User-Name"))
		photoURL := strings.TrimSpace(c.GetHeader("X-User-Photo"))

		user := &domain.User{
			FirebaseUID: fuid,
			Email:       email,
			DisplayName: &displayName,
			PhotoURL:    &photoURL,
			Role:        "user", // default role
			Preferences: make(map[string]interface{}),
		}

		if displayName == "" {
			user.DisplayName = nil
		}
		if photoURL == "" {
			user.PhotoURL = nil
		}

		if err := userRepo.Upsert(user); err != nil {
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
