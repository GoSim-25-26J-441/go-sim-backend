package http

import (
	"net/http"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/rag"
	"github.com/gin-gonic/gin"
)

func (h *Handler) ragSearch(c *gin.Context) {
	q := c.Query("q")
	if q == "" {
		c.JSON(400, gin.H{"ok": false, "error": "missing q"})
		return
	}
	results := rag.Search(q)
	c.JSON(200, gin.H{"ok": true, "results": results})
}

func (h *Handler) ragReload(c *gin.Context) {
	dir := c.DefaultQuery("dir", "internal/design_input_processing/rag/snippets")

	if err := rag.Load(dir); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error(), "dir": dir})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "dir": dir})
}
