package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/projects/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/projects/utils"
	"github.com/gin-gonic/gin"
)

type createDiagramReq struct {
	Source         string          `json:"source,omitempty"`
	DiagramJSON    json.RawMessage `json:"diagram_json"`
	ImageObjectKey string          `json:"image_object_key,omitempty"`
	SpecSummary    json.RawMessage `json:"spec_summary,omitempty"`
	Hash           string          `json:"hash,omitempty"`
}

func (h *Handler) createVersion(c *gin.Context) {
	publicID := strings.TrimSpace(c.Param("public_id"))
	if publicID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "missing project id"})
		return
	}

	var req createDiagramReq
	if err := c.ShouldBindJSON(&req); err != nil || len(req.DiagramJSON) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid body"})
		return
	}

	fuid := strings.TrimSpace(c.GetString("firebase_uid"))
	if fuid == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "missing user"})
		return
	}

	ver, err := h.diagramService.CreateVersion(c.Request.Context(), fuid, publicID, domain.CreateVersionInput{
		Source:         strings.TrimSpace(req.Source),
		DiagramJSON:    req.DiagramJSON,
		ImageObjectKey: strings.TrimSpace(req.ImageObjectKey),
		SpecSummary:    req.SpecSummary,
		Hash:           strings.TrimSpace(req.Hash),
	})
	if err != nil {
		if err == domain.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "project not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"ok": true, "diagram_version": ver})
}

// uploadDiagramImage uploads a diagram image (PNG/JPG) to S3 and returns an image_object_key
// that can be passed to the /:public_id/diagram endpoint.
func (h *Handler) uploadDiagramImage(c *gin.Context) {
	if h.s3Client == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": "image upload is disabled (S3 not configured)"})
		return
	}

	publicID := strings.TrimSpace(c.Param("public_id"))
	if publicID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "missing project id"})
		return
	}

	fuid := strings.TrimSpace(c.GetString("firebase_uid"))
	if fuid == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "missing user"})
		return
	}

	// Ensure project exists and belongs to user
	if _, _, err := h.projectService.GetByPublicID(c.Request.Context(), fuid, publicID); err != nil {
		if err == domain.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "project not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "missing file"})
		return
	}

	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "unsupported file type (only .png, .jpg, .jpeg)"})
		return
	}

	src, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "failed to open uploaded file"})
		return
	}
	defer src.Close()

	data, err := io.ReadAll(src)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "failed to read uploaded file"})
		return
	}

	imgID, err := utils.NewID("dimg")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "failed to generate image key"})
		return
	}

	// Store under arcfind-includes/diagrams in the S3 bucket, grouped by project id.
	key := fmt.Sprintf("arcfind-includes/diagrams/%s/%s%s", publicID, imgID, ext)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	if err := h.s3Client.PutObject(ctx, key, data); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "failed to upload image to storage"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"ok":              true,
		"image_object_key": key,
	})
}

func (h *Handler) latest(c *gin.Context) {
	publicID := strings.TrimSpace(c.Param("public_id"))
	if publicID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "missing project id"})
		return
	}

	fuid := strings.TrimSpace(c.GetString("firebase_uid"))
	if fuid == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "missing user"})
		return
	}

	ver, err := h.diagramService.GetLatest(c.Request.Context(), fuid, publicID)
	if err != nil {
		if err == domain.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "no diagram found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "diagram_version": ver})
}
