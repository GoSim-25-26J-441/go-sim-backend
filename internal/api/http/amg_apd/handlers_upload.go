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
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/utils"
)

type nodeLayoutXY struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

func mergeNodeLayoutIntoGraph(g *domain.Graph, layout map[string]nodeLayoutXY) {
	if g == nil || len(layout) == 0 {
		return
	}
	for id, pos := range layout {
		n := g.Nodes[id]
		if n == nil {
			continue
		}
		x, y := pos.X, pos.Y
		n.X = &x
		n.Y = &y
	}
}

func getIncomingDir() string {
	if d := os.Getenv("AMG_APD_INCOMING_DIR"); d != "" {
		return d
	}
	return "/app/incoming"
}

type analyzeRawReq struct {
	YAML                  string                  `json:"yaml"`
	Title                 string                  `json:"title"`
	OutDir                string                  `json:"out_dir"`
	NodeLayout            map[string]nodeLayoutXY `json:"node_layout,omitempty"`
	MergePreviousDiagram  *bool                   `json:"merge_previous_diagram,omitempty"`
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
	mergeNodeLayoutIntoGraph(res.Graph, req.NodeLayout)
	graphJSON, _ := json.Marshal(res.Graph)
	detectionsJSON, _ := json.Marshal(res.Detections)
	mergePrev := true
	if req.MergePreviousDiagram != nil {
		mergePrev = *req.MergePreviousDiagram
	}
	row, err := h.versionRepo.Save(userID, chatID, req.Title, req.YAML, graphJSON, detectionsJSON, dotContent, mergePrev)
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
	mergePrev := true
	if v := strings.TrimSpace(c.PostForm("merge_previous_diagram")); v == "0" || strings.EqualFold(v, "false") {
		mergePrev = false
	}
	row, err := h.versionRepo.Save(userID, chatID, title, string(yamlBytes), graphJSON, detectionsJSON, dotContent, mergePrev)
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

type updateVersionAnalysisReq struct {
	VersionID  string                  `json:"version_id"`
	YAML       string                  `json:"yaml"`
	NodeLayout map[string]nodeLayoutXY `json:"node_layout,omitempty"`
}

// UpdateVersionAnalysis runs analysis and updates an existing diagram version in place (no new version).
// Used when "Check Anti-Patterns" from chat has needs_analysis and version_id.
func (h *Handlers) UpdateVersionAnalysis(c *gin.Context) {
	var req updateVersionAnalysisReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "version_id is required"})
		return
	}
	if req.VersionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "version_id is required"})
		return
	}
	userID := getUserID(c)
	chatID := getChatID(c)

	row, err := h.versionRepo.GetByIDForUserProject(req.VersionID, userID, chatID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load version", "details": err.Error()})
		return
	}
	if row == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "version not found"})
		return
	}
	yamlContent := req.YAML
	if yamlContent == "" {
		yamlContent = row.YAMLContent
	}
	if yamlContent == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no yaml content"})
		return
	}
	title := row.Title
	if title == "" {
		title = "From diagram"
	}

	res, dotContent, errAnalyze := service.AnalyzeYAMLBytesInMemory([]byte(yamlContent), title, os.Getenv("DOT_BIN"))
	if errAnalyze != nil {
		c.String(http.StatusBadRequest, fmt.Sprintf("analyze failed: %v", errAnalyze))
		return
	}
	mergeNodeLayoutIntoGraph(res.Graph, req.NodeLayout)
	graphJSON, _ := json.Marshal(res.Graph)
	detectionsJSON, _ := json.Marshal(res.Detections)
	// Only merge into existing diagram_json when re-analyzing the same YAML already stored on this row.
	// If the client sends different YAML (e.g. stale version_id + suggestion-fixed YAML), merging would
	// splice the new model onto the old canvas and corrupt the historic version.
	normalizeYAML := func(s string) string {
		return strings.ReplaceAll(strings.TrimSpace(s), "\r\n", "\n")
	}
	preserveCanvasMerge := normalizeYAML(yamlContent) == normalizeYAML(row.YAMLContent)
	if err := h.versionRepo.UpdateDiagramVersionAnalysisByID(req.VersionID, userID, chatID, graphJSON, detectionsJSON, dotContent, yamlContent, preserveCanvasMerge); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update version", "details": err.Error()})
		return
	}
	updated, _ := h.versionRepo.GetByIDForUserProject(req.VersionID, userID, chatID)
	if updated == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "version not found after update"})
		return
	}
	var graph interface{}
	var detections interface{}
	_ = json.Unmarshal(updated.GraphJSON, &graph)
	_ = json.Unmarshal(updated.DetectionsJSON, &detections)
	c.JSON(http.StatusOK, gin.H{
		"graph":          graph,
		"detections":     detections,
		"dot_content":    updated.DOTContent,
		"version_id":     updated.ID,
		"version_number": updated.VersionNumber,
		"created_at":     updated.CreatedAt,
		"yaml_content":   updated.YAMLContent,
		"title":          updated.Title,
	})
}
