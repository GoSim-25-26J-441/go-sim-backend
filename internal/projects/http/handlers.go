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

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

