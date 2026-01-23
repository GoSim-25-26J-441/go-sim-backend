package http

import (
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/_legacy_http/handlers"
	"github.com/gin-gonic/gin"
)

func (h *Handler) intermediate(c *gin.Context) { handlers.Intermediate(c, h.UpstreamURL) }
func (h *Handler) fuse(c *gin.Context)         { handlers.Fuse(c, h.UpstreamURL) }
func (h *Handler) export(c *gin.Context)       { handlers.Export(c, h.UpstreamURL) }
func (h *Handler) report(c *gin.Context)       { handlers.Report(c, h.UpstreamURL) }

func (h *Handler) listJobsForUser(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		userID = c.GetHeader("X-User-Id")
		if userID == "" {
			userID = "demo-user"
		}
	}

	ids, err := handlers.ListJobsForUser(userID)
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

	summaries, err := handlers.JobSummariesForUser(h.UpstreamURL, userID)
	if err != nil {
		c.JSON(500, gin.H{"ok": false, "error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"ok": true, "jobs": summaries})
}
