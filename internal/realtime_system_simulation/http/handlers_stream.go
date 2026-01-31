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

	// Subscribe to Redis Pub/Sub for real-time updates
	// Channel format: sim:events:{run_id}
	eventChannel := fmt.Sprintf("sim:events:%s", runID)
	pubsub := h.redisClient.Subscribe(ctx, eventChannel)
	defer pubsub.Close()

	// Set up keep-alive pings
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	// Channel for pubsub messages with larger buffer to prevent drops
	// Default is 100, increase to 1000 to handle high-frequency metric updates
	pubsubChannel := pubsub.ChannelSize(1000)

	for {
		select {
		case <-ctx.Done():
			// Client disconnected
			return

		case <-ticker.C:
			// Send keep-alive ping
			fmt.Fprint(c.Writer, ": keep-alive\n\n")
			flusher.Flush()

		case msg := <-pubsubChannel:
			// Received update event from Redis Pub/Sub
			if msg == nil || msg.Payload == "" {
				continue
			}

			// Process message quickly to avoid blocking the channel
			var eventType string
			var eventJSON []byte

			// Try to parse as metric event or other event type first (more specific)
			// These are events from the metrics proxy (metric_update, status_change, etc.)
			var eventData map[string]interface{}
			if err := json.Unmarshal([]byte(msg.Payload), &eventData); err == nil {
				// Check if it's a metric_update or other event with "event" field
				if et, ok := eventData["event"].(string); ok {
					eventType = et
					eventJSON, _ = json.Marshal(eventData)
				}
			}

			// If not an event type, try to parse as run update
			if eventType == "" {
				var updatedRun domain.SimulationRun
				if err := json.Unmarshal([]byte(msg.Payload), &updatedRun); err == nil {
					// Validate that it's a valid run (has run_id and status)
					if updatedRun.RunID != "" && updatedRun.Status != "" {
						eventType = "update"
						eventJSON, _ = json.Marshal(gin.H{"run": updatedRun})
					} else {
						// Skip invalid/empty run updates silently to avoid log spam
						continue
					}
				} else {
					// Failed to parse - skip silently to avoid log spam
					continue
				}
			}

			// If we have a valid event, send it to the client
			if eventType != "" && len(eventJSON) > 0 {
				fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", eventType, string(eventJSON))
				flusher.Flush()
			}
		}
	}
}
