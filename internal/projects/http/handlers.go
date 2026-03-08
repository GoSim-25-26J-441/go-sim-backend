package http

import (
	"net/http"
	"strings"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/projects/domain"
	"github.com/gin-gonic/gin"
)

type createProjectReq struct {
	Name        string `json:"name"`
	IsTemporary bool   `json:"is_temporary"`
}

func (h *Handler) create(c *gin.Context) {
	var req createProjectReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid body"})
		return
	}

	userID := c.GetString("firebase_uid")
	p, err := h.projectService.Create(c.Request.Context(), userID, strings.TrimSpace(req.Name), req.IsTemporary)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"ok": true, "project": p})
}

func (h *Handler) list(c *gin.Context) {
	userID := c.GetString("firebase_uid")
	items, err := h.projectService.List(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "projects": items})
}

type renameProjectReq struct {
	Name string `json:"name"`
}

func (h *Handler) rename(c *gin.Context) {
	publicID := c.Param("public_id")

	var req renameProjectReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid body"})
		return
	}

	userID := c.GetString("firebase_uid")
	p, err := h.projectService.Rename(c.Request.Context(), userID, publicID, strings.TrimSpace(req.Name))
	if err != nil {
		if err == domain.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "project not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "project": p})
}

func (h *Handler) delete(c *gin.Context) {
	publicID := c.Param("public_id")
	userID := c.GetString("firebase_uid")

	ok, err := h.projectService.Delete(c.Request.Context(), userID, publicID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "project not found"})
		return
	}

	if h.OnProjectDeleted != nil {
		h.OnProjectDeleted(userID, publicID)
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) summary(c *gin.Context) {
	publicID := strings.TrimSpace(c.Param("public_id"))
	userID := c.GetString("firebase_uid")

	if publicID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "missing project id"})
		return
	}

	// Get project details with current_diagram_version_id
	project, currentDiagramVersionID, err := h.projectService.GetByPublicID(c.Request.Context(), userID, publicID)
	if err != nil {
		if err == domain.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "project not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}

	// Get all diagram versions
	allVersions, err := h.diagramService.ListAllVersions(c.Request.Context(), userID, publicID)
	if err != nil {
		if err == domain.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "project not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}

	// Find latest diagram version (first in the list since it's ordered DESC)
	var latestVersion *domain.DiagramVersion
	if len(allVersions) > 0 {
		latestVersion = &allVersions[0]
	}

	// Other versions (excluding the latest)
	otherVersions := []domain.DiagramVersion{}
	if len(allVersions) > 1 {
		otherVersions = allVersions[1:]
	}

	// It is only used temporarily when posting the first message
	var design map[string]interface{} = nil

	c.JSON(http.StatusOK, gin.H{
		"ok": true,
		"project": gin.H{
			"public_id":                  project.PublicID,
			"name":                       project.Name,
			"is_temporary":               project.Temporary,
			"current_diagram_version_id": currentDiagramVersionID,
			"created_at":                 project.CreatedAt,
			"updated_at":                 project.UpdatedAt,
		},
		"latest_diagram_version": latestVersion,
		"other_diagram_versions": otherVersions,
		"design":                 design,
	})
}
