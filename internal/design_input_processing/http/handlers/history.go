package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

func ReadChat(userID, jobID string, limit int) []chatTurn {
	baseDir := chatBaseDir(userID)
	fpath := filepath.Join(baseDir, fmt.Sprintf("chat-%s.jsonl", jobID))

	f, err := os.Open(fpath)
	if err != nil {
		// no file = no history
		return nil
	}
	defer f.Close()

	var turns []chatTurn
	dec := json.NewDecoder(f)
	for {
		var t chatTurn
		if err := dec.Decode(&t); err != nil {
			if err == io.EOF {
				break
			}
			// on decode error, just return what we have so far
			return turns
		}
		turns = append(turns, t)
	}

	if limit > 0 && len(turns) > limit {
		turns = turns[len(turns)-limit:]
	}
	return turns
}

// ClearChat removes the chat log for a given user + job.
func ClearChat(userID, jobID string) bool {
	baseDir := chatBaseDir(userID)
	fpath := filepath.Join(baseDir, fmt.Sprintf("chat-%s.jsonl", jobID))

	if err := os.Remove(fpath); err != nil && !os.IsNotExist(err) {
		return false
	}
	return true
}

// ChatHistory is the HTTP handler for GET /jobs/:id/chat/history
// It is now fully user-aware: it reads from the folder based on X-User-Id.
func ChatHistory(c *gin.Context) {
	jobID := c.Param("id")

	// same user resolution as Chat
	userID := c.GetString("user_id")
	if userID == "" {
		userID = c.GetHeader("X-User-Id")
	}
	if strings.TrimSpace(userID) == "" {
		userID = "demo-user"
	}

	history := ReadChat(userID, jobID, 0)
	if history == nil {
		// no file / no history yet is not an error
		history = []chatTurn{}
	}

	c.JSON(200, gin.H{"ok": true, "history": history})
}

// ChatClear is the HTTP handler for DELETE /jobs/:id/chat/history
func ChatClear(c *gin.Context) {
	jobID := c.Param("id")

	userID := c.GetString("user_id")
	if userID == "" {
		userID = c.GetHeader("X-User-Id")
	}
	if strings.TrimSpace(userID) == "" {
		userID = "demo-user"
	}

	if !ClearChat(userID, jobID) {
		c.JSON(500, gin.H{"ok": false, "error": "failed to clear chat"})
		return
	}

	c.JSON(200, gin.H{"ok": true})
}
