package amg_apd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/service"
)

// GetLatestForProject returns the latest AMG-APD version for a given project_public_id.
// If analysis fields are missing (graph/detections/dot), it analyzes the stored yaml_content
// and updates the existing row in diagram_versions (no new version created).
func (h *Handlers) GetLatestForProject(c *gin.Context) {
	projectPublicID := c.Param("project_public_id")
	if projectPublicID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project_public_id is required"})
		return
	}

	userID := getUserID(c)

	row, err := h.versionRepo.GetLatestByUserProject(userID, projectPublicID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load latest version", "details": err.Error()})
		return
	}
	if row == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no versions found for project"})
		return
	}
	if row.YAMLContent == "" {
		c.JSON(http.StatusConflict, gin.H{"error": "latest version has no yaml_content"})
		return
	}

	missing := len(row.GraphJSON) == 0 || len(row.DetectionsJSON) == 0 || row.DOTContent == ""
	if missing {
		res, dotContent, err := service.AnalyzeYAMLBytesInMemory([]byte(row.YAMLContent), row.Title, os.Getenv("DOT_BIN"))
		if err != nil {
			c.String(http.StatusBadRequest, fmt.Sprintf("analyze failed: %v", err))
			return
		}

		graphJSON, _ := json.Marshal(res.Graph)
		detectionsJSON, _ := json.Marshal(res.Detections)
		if err := h.versionRepo.UpdateAnalysisByID(row.ID, userID, projectPublicID, graphJSON, detectionsJSON, dotContent); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update version analysis", "details": err.Error()})
			return
		}

		row.GraphJSON = graphJSON
		row.DetectionsJSON = detectionsJSON
		row.DOTContent = dotContent
	}

	// Return same shape as analyze endpoints for frontend consumption.
	var graph interface{}
	var detections interface{}
	_ = json.Unmarshal(row.GraphJSON, &graph)
	_ = json.Unmarshal(row.DetectionsJSON, &detections)

	c.JSON(http.StatusOK, gin.H{
		"graph":          graph,
		"detections":     detections,
		"dot_content":    row.DOTContent,
		"dot_path":       "",
		"svg_path":       "",
		"version_id":     row.ID,
		"version_number": row.VersionNumber,
		"created_at":     row.CreatedAt,
		"yaml_content":   row.YAMLContent,
		"title":          row.Title,
	})
}

