package handlers

import (
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func Intermediate(c *gin.Context, upstreamURL string) {
	id := c.Param("id")
	url := upstreamURL + "/jobs/" + id + "/intermediate"

	req, err := http.NewRequestWithContext(c.Request.Context(), "GET", url, nil)
	if err != nil {
		c.JSON(502, gin.H{"ok": false, "error": err.Error()})
		return
	}

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
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

func Fuse(c *gin.Context, upstreamURL string) {
	id := c.Param("id")
	url := upstreamURL + "/jobs/" + id + "/fuse"

	req, err := http.NewRequestWithContext(c.Request.Context(), "POST", url, nil)
	if err != nil {
		c.JSON(502, gin.H{"ok": false, "error": err.Error()})
		return
	}

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

func Export(c *gin.Context, upstreamURL string) {
	id := c.Param("id")
	url := upstreamURL + "/jobs/" + id + "/export"
	if qs := c.Request.URL.RawQuery; qs != "" {
		url += "?" + qs
	}

	req, err := http.NewRequestWithContext(c.Request.Context(), "GET", url, nil)
	if err != nil {
		c.JSON(502, gin.H{"ok": false, "error": err.Error()})
		return
	}

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
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

func Report(c *gin.Context, upstreamURL string) {
	id := c.Param("id")
	url := upstreamURL + "/jobs/" + id + "/report"
	if qs := c.Request.URL.RawQuery; qs != "" {
		url += "?" + qs
	}

	req, err := http.NewRequestWithContext(c.Request.Context(), "GET", url, nil)
	if err != nil {
		c.JSON(502, gin.H{"ok": false, "error": err.Error()})
		return
	}

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
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
