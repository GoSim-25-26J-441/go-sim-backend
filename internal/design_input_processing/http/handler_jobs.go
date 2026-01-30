package http

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/service"
)

func (h *Handler) intermediate(c *gin.Context) {
	id := c.Param("id")

	resp, err := h.upstreamClient.GetIntermediate(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "error": err.Error()})
		return
	}
	defer resp.Body.Close()

	proxyResponse(c, resp)
}

func (h *Handler) fuse(c *gin.Context) {
	id := c.Param("id")
	userID := c.GetString("firebase_uid")

	resp, err := h.upstreamClient.Fuse(c.Request.Context(), id, userID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "error": err.Error()})
		return
	}
	defer resp.Body.Close()

	proxyResponse(c, resp)
}

func (h *Handler) export(c *gin.Context) {
	jobID := c.Param("id")

	// Get user ID from Firebase auth context
	userID := c.GetString("firebase_uid")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "user not authenticated"})
		return
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

	opts := service.ExportOptions{
		Format:   format,
		Download: download,
	}

	resp, err := h.upstreamClient.Export(c.Request.Context(), jobID, opts)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "error": err.Error()})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "error": "read response: " + err.Error()})
		return
	}

	if resp.StatusCode != http.StatusOK {
		proxyResponseWithBody(c, resp, body)
		return
	}

	// Enhance JSON export with signals if format is json
	if format == "json" {
		var spec map[string]any
		if err := json.Unmarshal(body, &spec); err == nil {
			signals := h.signalService.LoadSignalsFromHistory(jobID, userID)

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

	proxyResponseWithBody(c, resp, body)
}

func (h *Handler) report(c *gin.Context) {
	id := c.Param("id")
	query := c.Request.URL.RawQuery

	resp, err := h.upstreamClient.GetReport(c.Request.Context(), id, query)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "error": err.Error()})
		return
	}
	defer resp.Body.Close()

	proxyResponse(c, resp)
}

func (h *Handler) listJobsForUser(c *gin.Context) {
	userID := c.GetString("firebase_uid")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "user not authenticated"})
		return
	}

	ids, err := h.jobService.ListJobIDs(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "jobs": ids})
}

func (h *Handler) listJobsSummary(c *gin.Context) {
	userID := c.GetString("firebase_uid")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "user not authenticated"})
		return
	}

	summaries, err := h.jobService.GetJobSummaries(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "jobs": summaries})
}
