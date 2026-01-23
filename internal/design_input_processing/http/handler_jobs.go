package http

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
)

type JobSummary struct {
	ID           string `json:"id"`
	Services     int    `json:"services"`
	Dependencies int    `json:"dependencies"`
	Gaps         int    `json:"gaps"`
}

func (h *Handler) intermediate(c *gin.Context) {
	id := c.Param("id")
	url := h.UpstreamURL + "/jobs/" + id + "/intermediate"

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

func (h *Handler) fuse(c *gin.Context) {
	id := c.Param("id")
	url := h.UpstreamURL + "/jobs/" + id + "/fuse"

	req, err := http.NewRequestWithContext(c.Request.Context(), "POST", url, nil)
	if err != nil {
		c.JSON(502, gin.H{"ok": false, "error": err.Error()})
		return
	}

	if userID := c.GetHeader("X-User-Id"); userID != "" {
		req.Header.Set("X-User-Id", userID)
	}

	client := &http.Client{
		Timeout: 3 * time.Minute,
	}

	resp, err := client.Do(req)
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
	jobID := c.Param("id")

	// Resolve user ID (same pattern as chat / summaries)
	userID := c.GetString("user_id")
	if userID == "" {
		userID = c.GetHeader("X-User-Id")
		if userID == "" {
			userID = "demo-user"
		}
	}

	// Read query from incoming request
	format := c.Query("format")
	if format == "" {
		format = "json"
	}
	download := c.Query("download")
	if download == "" {
		download = "false"
	}

	// Build upstream URL safely
	u, err := url.Parse(h.UpstreamURL)
	if err != nil {
		c.JSON(502, gin.H{"ok": false, "error": "invalid upstream URL: " + err.Error()})
		return
	}
	u.Path = u.Path + "/jobs/" + jobID + "/export"

	q := u.Query()
	q.Set("format", format)
	q.Set("download", download)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, u.String(), nil)
	if err != nil {
		c.JSON(502, gin.H{"ok": false, "error": "build request: " + err.Error()})
		return
	}

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(502, gin.H{"ok": false, "error": "upstream export: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(502, gin.H{"ok": false, "error": "read upstream export: " + err.Error()})
		return
	}

	if resp.StatusCode != http.StatusOK {
		for k, vs := range resp.Header {
			if len(vs) > 0 {
				c.Header(k, vs[0])
			}
		}
		c.Status(resp.StatusCode)
		_, _ = c.Writer.Write(body)
		return
	}

	if format == "json" {
		var spec map[string]any
		if err := json.Unmarshal(body, &spec); err == nil {
			signals := h.loadSignalsFromHistory(jobID, userID)

			if len(signals) > 0 {
				sizing := map[string]any{}
				for k, v := range signals {
					if v != 0 {
						sizing[k] = v
					}
				}
				spec["sizing"] = sizing

				body, _ = json.MarshalIndent(spec, "", "  ")
				resp.Header.Set("Content-Type", "application/json; charset=utf-8")
			}
		}
	}

	for k, vs := range resp.Header {
		if len(vs) > 0 {
			c.Header(k, vs[0])
		}
	}
	c.Status(resp.StatusCode)
	_, _ = c.Writer.Write(body)
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

func (h *Handler) listJobsForUser(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		userID = c.GetHeader("X-User-Id")
		if userID == "" {
			userID = "demo-user"
		}
	}

	ids, err := listJobIDsForUser(userID)
	if err != nil {
		c.JSON(500, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true, "jobs": ids})
}

func (h *Handler) listJobsSummary(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		userID = c.GetHeader("X-User-Id")
		if userID == "" {
			userID = "demo-user"
		}
	}

	summaries, err := h.jobSummariesForUser(userID)
	if err != nil {
		c.JSON(500, gin.H{"ok": false, "error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"ok": true, "jobs": summaries})
}

func (h *Handler) summarizeJob(id string, ig, report map[string]any) JobSummary {
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

	// we don't have gaps here, so leave 0
	return js
}

func (h *Handler) jobSummariesForUser(userID string) ([]JobSummary, error) {
	ids, err := listJobIDsForUser(userID)
	if err != nil {
		return nil, err
	}

	out := make([]JobSummary, 0, len(ids))

	for _, id := range ids {
		ig, _ := h.fetchJSON(
			fmt.Sprintf("%s/jobs/%s/intermediate", h.UpstreamURL, id),
			5*time.Second,
		)
		report, _ := h.fetchJSON(
			fmt.Sprintf("%s/jobs/%s/report", h.UpstreamURL, id),
			5*time.Second,
		)

		out = append(out, h.summarizeJob(id, ig, report))
	}

	return out, nil
}
