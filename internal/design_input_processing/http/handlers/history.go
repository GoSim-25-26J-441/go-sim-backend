package handlers

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
)

func ReadChat(jobID string, limit int) []chatTurn {
	dir := os.Getenv("CHAT_LOG_DIR")
	if dir == "" {
		dir = filepath.FromSlash("D:/Research/go-sim-backend/internal/design_input_processing/data/chat_logs")
	}
	fpath := filepath.Join(dir, "chat-"+jobID+".jsonl")
	b, err := os.ReadFile(fpath)
	if err != nil {
		return nil
	}

	lines := bytes.Split(b, []byte("\n"))
	turns := make([]chatTurn, 0, len(lines))
	for _, ln := range lines {
		ln = bytes.TrimSpace(ln)
		if len(ln) == 0 {
			continue
		}
		var t chatTurn
		if json.Unmarshal(ln, &t) == nil {
			turns = append(turns, t)
		}
	}
	if limit > 0 && len(turns) > limit {
		turns = turns[len(turns)-limit:]
	}
	return turns
}

func ClearChat(jobID string) bool {
	dir := os.Getenv("CHAT_LOG_DIR")
	if dir == "" {
		dir = filepath.FromSlash("D:/Research/go-sim-backend/internal/design_input_processing/data/chat_logs")
	}
	fpath := filepath.Join(dir, "chat-"+jobID+".jsonl")
	if err := os.Remove(fpath); err != nil {
		return false
	}
	return true
}
