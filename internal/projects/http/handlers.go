package http

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type createReq struct {
	Name        string `json:"name"`
	IsTemporary bool   `json:"is_temporary"`
}

func (h *Handler) create(c *gin.Context) {
	var req createReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid body"})
		return
	}

	userID := c.GetString("firebase_uid")
	p, err := h.repo.Create(c.Request.Context(), userID, strings.TrimSpace(req.Name), req.IsTemporary)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"ok": true, "project": p})
}

func (h *Handler) list(c *gin.Context) {
	userID := c.GetString("firebase_uid")
	items, err := h.repo.List(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "projects": items})
}

type renameReq struct {
	Name string `json:"name"`
}

func (h *Handler) rename(c *gin.Context) {
	publicID := c.Param("public_id")

	var req renameReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid body"})
		return
	}

	userID := c.GetString("firebase_uid")
	p, err := h.repo.Rename(c.Request.Context(), userID, publicID, strings.TrimSpace(req.Name))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "project not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "project": p})
}

func (h *Handler) delete(c *gin.Context) {
	publicID := c.Param("public_id")
	userID := c.GetString("firebase_uid")

	ok, err := h.repo.SoftDelete(c.Request.Context(), userID, publicID)
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

