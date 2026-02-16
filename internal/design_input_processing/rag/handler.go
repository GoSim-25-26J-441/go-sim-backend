package rag

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler handles HTTP for the RAG pipeline.
type Handler struct{}

// NewHandler returns a new RAG handler.
func NewHandler() *Handler {
	return &Handler{}
}

// Search is a placeholder for RAG search (GET /rag or /rag/search).
func (h *Handler) Search(c *gin.Context) {
	q := c.Query("q")
	_ = q // use when implementing search
	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"message": "RAG pipeline placeholder - implement search here",
		"data":    []interface{}{},
	})
}

// Ingest is a placeholder for RAG ingest/index (POST /rag).
func (h *Handler) Ingest(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"message": "RAG pipeline placeholder - implement ingest here",
	})
}

// GetRequirementsQuestions returns the requirements questionnaire questions
func (h *Handler) GetRequirementsQuestions(c *gin.Context) {
	config, err := GetQuestions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"ok":    false,
			"error": fmt.Sprintf("failed to load questions: %v", err),
		})
		return
	}

	if !config.Enabled {
		c.JSON(http.StatusOK, gin.H{
			"ok":       true,
			"enabled":  false,
			"questions": []Question{},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":        true,
		"enabled":   true,
		"questions": config.Questions,
	})
}
