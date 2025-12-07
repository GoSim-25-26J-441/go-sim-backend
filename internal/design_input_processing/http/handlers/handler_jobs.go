package handlers

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type JobSummary struct {
	ID           string `json:"id"`
	Services     int    `json:"services"`
	Dependencies int    `json:"dependencies"`
	Gaps         int    `json:"gaps"`
}

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

func summarizeJob(id string, ig, report map[string]any) JobSummary {
	js := JobSummary{ID: id}

	// 1) Prefer the report counts if present
	if countsRaw, ok := report["counts"].(map[string]any); ok {
		if v, ok := countsRaw["services"].(float64); ok {
			js.Services = int(v)
		}
		if v, ok := countsRaw["dependencies"].(float64); ok {
			js.Dependencies = int(v)
		}
		if v, ok := countsRaw["gaps"].(float64); ok {
			js.Gaps = int(v)
		}
		return js
	}

	// 2) Fallback to intermediate graph if report not available
	if nodes, ok := ig["Nodes"].([]any); ok {
		js.Services = len(nodes)
	}
	if edges, ok := ig["Edges"].([]any); ok {
		js.Dependencies = len(edges)
	}

	// we don’t have gaps here, so leave 0
	return js
}

func JobSummariesForUser(upstreamURL, userID string) ([]JobSummary, error) {
	// reuse the function you already have
	ids, err := ListJobsForUser(userID)
	if err != nil {
		return nil, err
	}

	out := make([]JobSummary, 0, len(ids))

	for _, id := range ids {
		ig, _ := fetchJSON(
			fmt.Sprintf("%s/jobs/%s/intermediate", upstreamURL, id),
			5*time.Second,
		)
		report, _ := fetchJSON(
			fmt.Sprintf("%s/jobs/%s/report", upstreamURL, id),
			5*time.Second,
		)

		out = append(out, summarizeJob(id, ig, report))
	}

	return out, nil
}

func ListJobSummaries(c *gin.Context, upstreamURL string) {
	userID := c.GetString("user_id")
	if userID == "" {
		userID = c.GetHeader("X-User-Id")
		if userID == "" {
			userID = "demo-user"
		}
	}

	ids, err := ListJobsForUser(userID)
	if err != nil {
		c.JSON(500, gin.H{"ok": false, "error": err.Error()})
		return
	}

	summaries := make([]JobSummary, 0, len(ids))
	for _, id := range ids {
		rep, err := fetchJSON(fmt.Sprintf("%s/jobs/%s/report", upstreamURL, id), 5*time.Second)
		if err != nil {
			// just skip broken ones
			continue
		}
		counts, _ := rep["counts"].(map[string]any)

		summary := JobSummary{
			ID:           id,
			Services:     intFromMap(counts, "services"),
			Dependencies: intFromMap(counts, "dependencies"),
			Gaps:         intFromMap(counts, "gaps"),
		}
		summaries = append(summaries, summary)
	}

	c.JSON(200, gin.H{
		"ok":   true,
		"jobs": summaries,
	})
}
