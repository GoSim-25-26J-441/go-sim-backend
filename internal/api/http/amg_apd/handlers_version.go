package amg_apd

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/storage/amg_apd_version"
)

// userID and chatID from headers; fallback to placeholders
const defaultUserID = "TestUser123"
const defaultChatID = "TestChat123"

func getUserID(c *gin.Context) string {
	if v := strings.TrimSpace(c.GetHeader("X-User-Id")); v != "" {
		return v
	}
	return defaultUserID
}

func getChatID(c *gin.Context) string {
	if v := strings.TrimSpace(c.GetHeader("X-Chat-Id")); v != "" {
		return v
	}
	return defaultChatID
}

// ListVersions returns version summaries for the current user/chat.
func (h *Handlers) ListVersions(c *gin.Context) {
	userID := getUserID(c)
	chatID := getChatID(c)
	summaries, err := h.versionRepo.ListSummariesByUserChat(userID, chatID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list versions", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"versions": summaries})
}

// GetVersion returns a single version by id (must belong to user/chat).
func (h *Handlers) GetVersion(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "version id is required"})
		return
	}
	userID := getUserID(c)
	chatID := getChatID(c)
	row, err := h.versionRepo.GetByIDForUserChat(id, userID, chatID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get version", "details": err.Error()})
		return
	}
	if row == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "version not found"})
		return
	}
	var graph domain.Graph
	var detections []domain.Detection
	if err := amg_apd_version.ParseGraphAndDetections(row, &graph, &detections); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse version", "details": err.Error()})
		return
	}
	graph.RebuildOutIn()
	c.JSON(http.StatusOK, gin.H{
		"id":             row.ID,
		"version_number": row.VersionNumber,
		"title":          row.Title,
		"source":         row.Source,
		"yaml_content":   row.YAMLContent,
		"graph":          &graph,
		"dot_content":    row.DOTContent,
		"detections":     detections,
		"created_at":     row.CreatedAt,
	})
}

// PatchVersion updates mutable fields on a version (e.g. title). Any source row for this user/project.
func (h *Handlers) PatchVersion(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "version id is required"})
		return
	}
	var body struct {
		Title string `json:"title"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body"})
		return
	}
	if strings.TrimSpace(body.Title) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}
	userID := getUserID(c)
	chatID := getChatID(c)
	updated, err := h.versionRepo.UpdateTitleForUserChat(id, userID, chatID, body.Title)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if updated == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "version not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":             updated.ID,
		"version_number": updated.VersionNumber,
		"title":          updated.Title,
		"source":         updated.Source,
		"created_at":     updated.CreatedAt,
	})
}

// DeleteVersion deletes a version by id (must belong to user/chat).
func (h *Handlers) DeleteVersion(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "version id is required"})
		return
	}
	userID := getUserID(c)
	chatID := getChatID(c)
	ok, err := h.versionRepo.DeleteByIDForUserChat(id, userID, chatID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete version", "details": err.Error()})
		return
	}
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "version not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "deleted": id})
}

// CompareVersions returns two versions for side-by-side compare.
// Query: ?left=<id>&right=<id> or JSON body { "left_id": "", "right_id": "" }.
func (h *Handlers) CompareVersions(c *gin.Context) {
	var leftID, rightID string
	if c.GetHeader("Content-Type") == "application/json" {
		var body struct {
			LeftID  string `json:"left_id"`
			RightID string `json:"right_id"`
		}
		if err := c.ShouldBindJSON(&body); err == nil {
			leftID, rightID = body.LeftID, body.RightID
		}
	}
	if leftID == "" {
		leftID = c.Query("left")
	}
	if rightID == "" {
		rightID = c.Query("right")
	}
	if leftID == "" || rightID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "left_id and right_id (or query left & right) are required"})
		return
	}
	userID := getUserID(c)
	chatID := getChatID(c)
	left, err := h.versionRepo.GetByIDForUserChat(leftID, userID, chatID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get left version", "details": err.Error()})
		return
	}
	if left == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "left version not found"})
		return
	}
	right, err := h.versionRepo.GetByIDForUserChat(rightID, userID, chatID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get right version", "details": err.Error()})
		return
	}
	if right == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "right version not found"})
		return
	}
	var leftGraph, rightGraph domain.Graph
	var leftDet, rightDet []domain.Detection
	_ = amg_apd_version.ParseGraphAndDetections(left, &leftGraph, &leftDet)
	_ = amg_apd_version.ParseGraphAndDetections(right, &rightGraph, &rightDet)
	leftGraph.RebuildOutIn()
	rightGraph.RebuildOutIn()
	c.JSON(http.StatusOK, gin.H{
		"left": gin.H{
			"id":             left.ID,
			"version_number": left.VersionNumber,
			"title":          left.Title,
			"source":         left.Source,
			"yaml_content":   left.YAMLContent,
			"graph":          &leftGraph,
			"dot_content":    left.DOTContent,
			"detections":     leftDet,
			"created_at":     left.CreatedAt,
		},
		"right": gin.H{
			"id":             right.ID,
			"version_number": right.VersionNumber,
			"title":          right.Title,
			"source":         right.Source,
			"yaml_content":   right.YAMLContent,
			"graph":          &rightGraph,
			"dot_content":    right.DOTContent,
			"detections":     rightDet,
			"created_at":     right.CreatedAt,
		},
	})
}

// Handlers holds dependencies for AMG-APD HTTP handlers (e.g. version repo).
type Handlers struct {
	versionRepo *amg_apd_version.Repo
}

// NewHandlers builds AMG-APD handlers with the given version repo.
func NewHandlers(versionRepo *amg_apd_version.Repo) *Handlers {
	return &Handlers{versionRepo: versionRepo}
}

