package handlers

import (
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func Ingest(c *gin.Context, upstreamURL string) {
	req, err := http.NewRequestWithContext(c.Request.Context(),
		"POST", upstreamURL+"/ingest", c.Request.Body)
	if err != nil {
		c.JSON(502, gin.H{"ok": false, "error": err.Error()})
		return
	}
	req.Header = c.Request.Header.Clone()

	resp, err := (&http.Client{Timeout: 90 * time.Second}).Do(req)
	if err != nil {
		c.JSON(502, gin.H{"ok": false, "error": err.Error()})
		return
	}
	defer resp.Body.Close()

	for k, v := range resp.Header {
		if len(v) > 0 {
			c.Header(k, v[0])
		}
	}
	c.Status(resp.StatusCode)
	_, _ = io.Copy(c.Writer, resp.Body)
}
