package auth

import (
	"net/http"
	"strings"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/users"
	"github.com/gin-gonic/gin"
)

const (
	CtxFirebaseUID = "firebase_uid"
	CtxUserDBID    = "user_db_id"
)

func WithUser(userRepo *users.Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		fuid := strings.TrimSpace(c.GetHeader("X-User-Id"))
		if fuid == "" {
			fuid = "demo-user"
		}

		uid, err := userRepo.EnsureUser(c.Request.Context(), users.UpsertUser{
			FirebaseUID: fuid,
			Email:       c.GetHeader("X-User-Email"),
			DisplayName: c.GetHeader("X-User-Name"),
			PhotoURL:    c.GetHeader("X-User-Photo"),
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "ensure user: " + err.Error()})
			c.Abort()
			return
		}

		c.Set(CtxFirebaseUID, fuid)
		c.Set(CtxUserDBID, uid)
		c.Next()
	}
}

func UserDBID(c *gin.Context) string {
	v := c.GetString(CtxUserDBID)
	if strings.TrimSpace(v) == "" {
		return ""
	}
	return v
}
