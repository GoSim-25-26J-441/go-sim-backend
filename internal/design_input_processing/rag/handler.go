package rag

import (
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
