package http

import (
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	UpstreamURL string
}

func New(upstreamURL string) *Handler { return &Handler{UpstreamURL: upstreamURL} }

func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.GET("/health", h.health)
	rg.POST("/ingest", h.ingest)
	rg.GET("/jobs/:id/intermediate", h.intermediate)
	rg.POST("/jobs/:id/fuse", h.fuse)
	rg.GET("/jobs/:id/export", h.export)
	rg.GET("/jobs/:id/report", h.report)
}

func (h *Handler) health(c *gin.Context) {
	c.JSON(200, gin.H{
		"ok":        true,
		"component": "design-input-processing",
		"upstream":  h.UpstreamURL,
	})
}

func (h *Handler) ingest(c *gin.Context) {
	// forward the original multipart form to upstream /ingest
	req, err := http.NewRequestWithContext(c.Request.Context(), "POST", h.UpstreamURL+"/ingest", c.Request.Body)
	if err != nil {
		c.JSON(502, gin.H{"ok": false, "error": err.Error()})
		return
	}
	req.Header = c.Request.Header.Clone()

	cli := &http.Client{Timeout: 90 * time.Second}
	resp, err := cli.Do(req)
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
	io.Copy(c.Writer, resp.Body)
}

func (h *Handler) intermediate(c *gin.Context) {
	id := c.Param("id")
	url := h.UpstreamURL + "/jobs/" + id + "/intermediate"

	req, err := http.NewRequestWithContext(c.Request.Context(), "GET", url, nil)
	if err != nil {
		c.JSON(502, gin.H{"ok": false, "error": err.Error()})
		return
	}

	cli := &http.Client{Timeout: 30 * time.Second}
	resp, err := cli.Do(req)
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

func (h *Handler) fuse(c *gin.Context) {
	id := c.Param("id")
	url := h.UpstreamURL + "/jobs/" + id + "/fuse"

	req, err := http.NewRequestWithContext(c.Request.Context(), "POST", url, nil)
	if err != nil {
		c.JSON(502, gin.H{"ok": false, "error": err.Error()})
		return
	}

	cli := &http.Client{Timeout: 90 * time.Second}
	resp, err := cli.Do(req)
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

func (h *Handler) export(c *gin.Context) {
	id := c.Param("id")
	// pass through query string (format, download, etc.)
	url := h.UpstreamURL + "/jobs/" + id + "/export" + "?" + c.Request.URL.RawQuery

	req, err := http.NewRequestWithContext(c.Request.Context(), "GET", url, nil)
	if err != nil {
		c.JSON(502, gin.H{"ok": false, "error": err.Error()})
		return
	}

	cli := &http.Client{Timeout: 30 * time.Second}
	resp, err := cli.Do(req)
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

func (h *Handler) report(c *gin.Context) {
	id := c.Param("id")
	url := h.UpstreamURL + "/jobs/" + id + "/report"
	if qs := c.Request.URL.RawQuery; qs != "" {
		url += "?" + qs
	}

	req, err := http.NewRequestWithContext(c.Request.Context(), "GET", url, nil)
	if err != nil {
		c.JSON(502, gin.H{"ok": false, "error": err.Error()})
		return
	}

	cli := &http.Client{Timeout: 30 * time.Second}
	resp, err := cli.Do(req)
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
