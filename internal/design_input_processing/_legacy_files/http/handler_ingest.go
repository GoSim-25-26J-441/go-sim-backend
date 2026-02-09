package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *Handler) ingest(c *gin.Context) {
	resp, err := h.upstreamClient.Ingest(c.Request.Context(), c.Request.Body, c.Request.Header)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "error": err.Error()})
		return
	}
	defer resp.Body.Close()

	proxyResponse(c, resp)
}
