package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *Handler) graphviz(c *gin.Context) {
	jobID := c.Param("id")

	dotBytes, err := h.graphService.GetGraphvizDOT(c.Request.Context(), jobID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "error": err.Error()})
		return
	}

	c.Header("Content-Type", "text/vnd.graphviz; charset=utf-8")
	c.Data(http.StatusOK, "text/vnd.graphviz; charset=utf-8", dotBytes)
}
