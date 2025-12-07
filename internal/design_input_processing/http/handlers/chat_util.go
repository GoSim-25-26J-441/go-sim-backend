package handlers

import (
	"os"
	"path/filepath"
	"strings"
)

func chatBaseDir(userID string) string {
	dir := os.Getenv("CHAT_LOG_DIR")
	if dir == "" {
		dir = filepath.FromSlash("D:/Research/go-sim-backend/internal/design_input_processing/data/chat_logs")
	}

	if strings.TrimSpace(userID) == "" {
		userID = "demo-user"
	}

	userID = strings.ReplaceAll(userID, "..", "_")
	userID = strings.ReplaceAll(userID, "/", "_")
	userID = strings.ReplaceAll(userID, "\\", "_")

	return filepath.Join(dir, userID)
}
