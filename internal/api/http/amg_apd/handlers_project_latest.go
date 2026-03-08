package amg_apd

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/service"
)

// GetLatestForProject returns the latest AMG-APD version for a given project_public_id.
// If no AMG-APD version exists, tries to get the latest diagram YAML (any source) for this
// user+project, runs analysis, saves a new AMG-APD version, and returns it.
// If analysis fields are missing (graph/detections/dot) on an existing row, it analyzes
// the stored yaml_content and updates the row (no new version created).
func (h *Handlers) GetLatestForProject(c *gin.Context) {
	projectPublicID := c.Param("project_public_id")
	if projectPublicID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project_public_id is required"})
		return
	}

	userID := getUserID(c)
	chatID := getChatID(c)

	row, err := h.versionRepo.GetLatestByUserProject(userID, projectPublicID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load latest version", "details": err.Error()})
		return
	}

	if row == nil {
		// No AMG-APD version: try latest diagram YAML (any source), analyze, and save.
		yamlContent, title, errYAML := h.versionRepo.GetLatestYAMLByUserProject(userID, projectPublicID)
		if errYAML != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load project yaml", "details": errYAML.Error()})
			return
		}
		if yamlContent == "" {
			c.JSON(http.StatusNotFound, gin.H{"error": "no versions found for project"})
			return
		}
		if title == "" {
			title = "From diagram"
		}
		res, dotContent, errAnalyze := service.AnalyzeYAMLBytesInMemory([]byte(yamlContent), title, os.Getenv("DOT_BIN"))
		if errAnalyze != nil {
			// YAML may be incompatible with parser (e.g. different format); let frontend
			// re-submit via analyze-upload so it goes through the same flow as "Analyze & Visualize".
			c.JSON(http.StatusOK, gin.H{
				"needs_analysis": true,
				"yaml_content":   yamlContent,
				"title":          title,
			})
			return
		}
		graphJSON, _ := json.Marshal(res.Graph)
		detectionsJSON, _ := json.Marshal(res.Detections)
		row, err = h.versionRepo.Save(userID, chatID, title, yamlContent, graphJSON, detectionsJSON, dotContent)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save version", "details": err.Error()})
			return
		}
		// Return the new row in the same shape as below.
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
			// Analysis failed (e.g. yaml unmarshal error); return yaml so frontend
			// can re-submit via analyze-upload like "Analyze & Visualize".
			c.JSON(http.StatusOK, gin.H{
				"needs_analysis": true,
				"yaml_content":   row.YAMLContent,
				"title":          row.Title,
			})
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

