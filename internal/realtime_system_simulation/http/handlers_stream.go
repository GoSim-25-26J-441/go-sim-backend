package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/domain"
	"github.com/gin-gonic/gin"
)

// StreamRunEvents streams real-time updates for a simulation run using Server-Sent Events (SSE)
func (h *Handler) StreamRunEvents(c *gin.Context) {
	runID := c.Param("id")
	if runID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "run ID is required"})
		return
	}

	// Verify the run exists and user has access
	run, err := h.simService.GetRun(runID)
	if err != nil {
		if err == domain.ErrRunNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get run"})
		return
	}

	// Get user ID from context for authorization check
	userID := c.GetString("firebase_uid")
	if userID == "" {
		userID = c.GetHeader("X-User-Id")
	}
	if userID == "" || run.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // nginx: disable buffering

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming unsupported"})
		return
	}

	// Send initial run state
	initialData, _ := json.Marshal(gin.H{"run": run})
	fmt.Fprintf(c.Writer, "event: initial\ndata: %s\n\n", string(initialData))
	flusher.Flush()

	// Get context for cancellation
	ctx := c.Request.Context()

	// Set up keep-alive pings
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	// Poll for updates (checks every second for changes)
	pollTicker := time.NewTicker(1 * time.Second)
	defer pollTicker.Stop()

	lastUpdatedAt := run.UpdatedAt

	for {
		select {
		case <-ctx.Done():
			// Client disconnected
			return

		case <-ticker.C:
			// Send keep-alive ping
			fmt.Fprint(c.Writer, ": keep-alive\n\n")
			flusher.Flush()

		case <-pollTicker.C:
			// Poll for updates
			updatedRun, err := h.simService.GetRun(runID)
			if err != nil {
				// Run might have been deleted
				if err == domain.ErrRunNotFound {
					eventData, _ := json.Marshal(gin.H{"event": "deleted", "run_id": runID})
					fmt.Fprintf(c.Writer, "event: deleted\ndata: %s\n\n", string(eventData))
					flusher.Flush()
					return
				}
				continue
			}

			// Check if run was updated
			if updatedRun.UpdatedAt.After(lastUpdatedAt) {
				lastUpdatedAt = updatedRun.UpdatedAt

				// Send update event
				eventData, _ := json.Marshal(gin.H{"run": updatedRun})
				fmt.Fprintf(c.Writer, "event: update\ndata: %s\n\n", string(eventData))
				flusher.Flush()
			}
		}
	}
}
