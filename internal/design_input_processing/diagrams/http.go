package diagrams

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/auth"
	"github.com/gin-gonic/gin"
)

type handler struct {
	repo *Repo
}

type createReq struct {
	Source         string          `json:"source,omitempty"`
	DiagramJSON    json.RawMessage `json:"diagram_json"`
	ImageObjectKey string          `json:"image_object_key,omitempty"`
	SpecSummary    json.RawMessage `json:"spec_summary,omitempty"`
	Hash           string          `json:"hash,omitempty"`
}

func (h *handler) createVersion(c *gin.Context) {
	publicID := strings.TrimSpace(c.Param("public_id"))
	if publicID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "missing project id"})
		return
	}

	var req createReq
	if err := c.ShouldBindJSON(&req); err != nil || len(req.DiagramJSON) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid body"})
		return
	}

	fuid := strings.TrimSpace(auth.UserFirebaseUID(c))
	if fuid == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "missing user"})
		return
	}

	ver, err := h.repo.CreateVersion(c.Request.Context(), fuid, publicID, CreateVersionInput{
		Source:         strings.TrimSpace(req.Source),
		DiagramJSON:    req.DiagramJSON,
		ImageObjectKey: strings.TrimSpace(req.ImageObjectKey),
		SpecSummary:    req.SpecSummary,
		Hash:           strings.TrimSpace(req.Hash),
	})
	if err != nil {
		if err == ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "project not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"ok": true, "diagram_version": ver})
}

func (h *handler) latest(c *gin.Context) {
	publicID := strings.TrimSpace(c.Param("public_id"))
	if publicID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "missing project id"})
		return
	}

	fuid := strings.TrimSpace(auth.UserFirebaseUID(c))
	if fuid == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "missing user"})
		return
	}

	ver, err := h.repo.Latest(c.Request.Context(), fuid, publicID)
	if err != nil {
		if err == ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "no diagram found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "diagram_version": ver})
}
