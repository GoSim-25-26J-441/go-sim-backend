package http

import (
	"strings"

	"github.com/gin-gonic/gin"
)

func (h *Handler) chatHistory(c *gin.Context) {
	jobID := c.Param("id")

	userID := c.GetString("user_id")
	if userID == "" {
		userID = c.GetHeader("X-User-Id")
	}
	if strings.TrimSpace(userID) == "" {
		userID = "demo-user"
	}

	history := readChat(userID, jobID, 0)
	if history == nil {
		history = []chatTurn{}
	}

	c.JSON(200, gin.H{"ok": true, "history": history})
}

func (h *Handler) chatClear(c *gin.Context) {
	jobID := c.Param("id")

	userID := c.GetString("user_id")
	if userID == "" {
		userID = c.GetHeader("X-User-Id")
	}
	if strings.TrimSpace(userID) == "" {
		userID = "demo-user"
	}

	if !clearChat(userID, jobID) {
		c.JSON(500, gin.H{"ok": false, "error": "failed to clear chat"})
		return
	}

	c.JSON(200, gin.H{"ok": true})
}
