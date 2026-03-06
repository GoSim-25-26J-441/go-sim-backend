package amg_apd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/service"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/utils"
)

func getIncomingDir() string {
	if d := os.Getenv("AMG_APD_INCOMING_DIR"); d != "" {
		return d
	}
	return "/app/incoming"
}

type analyzeRawReq struct {
	YAML   string `json:"yaml"`
	Title  string `json:"title"`
	OutDir string `json:"out_dir"`
}

// AnalyzeRaw runs analysis and persists to DB (user_id/chat_id from headers or TestUser123/TestChat123).
func (h *Handlers) AnalyzeRaw(c *gin.Context) {
	var req analyzeRawReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.String(http.StatusBadRequest, "invalid json body")
		return
	}
	if req.YAML == "" {
		c.String(http.StatusBadRequest, "yaml is required")
		return
	}
	if req.Title == "" {
		req.Title = "Uploaded"
	}
	userID := getUserID(c)
	chatID := getChatID(c)

	res, dotContent, err := service.AnalyzeYAMLBytesInMemory([]byte(req.YAML), req.Title, os.Getenv("DOT_BIN"))
	if err != nil {
		c.String(http.StatusBadRequest, fmt.Sprintf("analyze failed: %v", err))
		return
	}
	graphJSON, _ := json.Marshal(res.Graph)
	detectionsJSON, _ := json.Marshal(res.Detections)
	row, err := h.versionRepo.Save(userID, chatID, req.Title, req.YAML, graphJSON, detectionsJSON, dotContent)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save version", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"graph":        res.Graph,
		"detections":   res.Detections,
		"dot_content":  dotContent,
		"dot_path":     "",
		"svg_path":     "",
		"version_id":   row.ID,
		"version_number": row.VersionNumber,
		"created_at":   row.CreatedAt,
	})
}

// AnalyzeUpload runs analysis on uploaded file and persists to DB.
func (h *Handlers) AnalyzeUpload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.String(http.StatusBadRequest, "file is required")
		return
	}
	title := c.PostForm("title")
	if title == "" {
		base := filepath.Base(file.Filename)
		title = strings.TrimSuffix(base, filepath.Ext(base))
		if title == "" {
			title = "Uploaded"
		}
	}
	userID := getUserID(c)
	chatID := getChatID(c)

	incoming := getIncomingDir()
	_ = os.MkdirAll(incoming, 0o755)
	ext := filepath.Ext(file.Filename)
	if ext == "" {
		ext = ".yaml"
	}
	tmpPath := filepath.Join(incoming, utils.NewID()+ext)
	if err := c.SaveUploadedFile(file, tmpPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("saving uploaded file failed: %v", err)})
		return
	}
	defer os.Remove(tmpPath)

	yamlBytes, err := os.ReadFile(tmpPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("read file failed: %v", err)})
		return
	}
	res, dotContent, err := service.AnalyzeYAMLBytesInMemory(yamlBytes, title, os.Getenv("DOT_BIN"))
	if err != nil {
		c.String(http.StatusBadRequest, fmt.Sprintf("analyze failed: %v", err))
		return
	}
	graphJSON, _ := json.Marshal(res.Graph)
	detectionsJSON, _ := json.Marshal(res.Detections)
	row, err := h.versionRepo.Save(userID, chatID, title, string(yamlBytes), graphJSON, detectionsJSON, dotContent)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save version", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"graph":          res.Graph,
		"detections":     res.Detections,
		"dot_content":    dotContent,
		"dot_path":       "",
		"svg_path":       "",
		"version_id":     row.ID,
		"version_number": row.VersionNumber,
		"created_at":     row.CreatedAt,
	})
}
