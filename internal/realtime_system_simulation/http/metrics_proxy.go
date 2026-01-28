package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/domain"
)

// StartMetricsStreamProxy subscribes to the simulator's metrics stream and forwards events to Redis Pub/Sub
// This allows the frontend to receive real-time metrics via SSE
// intervalMs: update interval in milliseconds (default 1000 if 0)
func (h *Handler) StartMetricsStreamProxy(runID string, engineRunID string, intervalMs int) {
	ctx, cancel := context.WithCancel(context.Background())

	// Default interval to 1000ms if not specified
	if intervalMs <= 0 {
		intervalMs = 1000
	}

	go func() {
		defer cancel()

		// Subscribe to simulator's metrics stream
		eventsChan, err := h.engineClient.StreamMetrics(engineRunID, intervalMs, ctx)
		if err != nil {
			log.Printf("Failed to subscribe to metrics stream for run_id=%s, engine_run_id=%s: %v", runID, engineRunID, err)
			return
		}

		log.Printf("Subscribed to metrics stream for run_id=%s, engine_run_id=%s (interval_ms=%d)", runID, engineRunID, intervalMs)

		// Forward events to Redis Pub/Sub
		eventChannel := fmt.Sprintf("sim:events:%s", runID)

		// Set up periodic check for terminal state (every 5 seconds)
		statusCheckTicker := time.NewTicker(5 * time.Second)
		defer statusCheckTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Printf("Metrics stream proxy stopped for run_id=%s", runID)
				return

			case <-statusCheckTicker.C:
				// Periodically check if the run has reached a terminal state
				run, err := h.simService.GetRun(runID)
				if err == nil {
					// Stop proxy if run is in terminal state
					if run.Status == domain.StatusCompleted ||
						run.Status == domain.StatusFailed ||
						run.Status == domain.StatusCancelled {
						log.Printf("Run %s reached terminal state (%s), stopping metrics proxy", runID, run.Status)
						cancel() // Cancel context to stop the proxy
						return
					}
				}

			case sseEvent, ok := <-eventsChan:
				if !ok {
					// Channel closed - stream ended
					log.Printf("Metrics stream ended for run_id=%s", runID)
					return
				}

				// Reduced logging - only log every 10th event or on errors to avoid spam
				// Uncomment for full debugging:
				// log.Printf("Received SSE event from simulator: type=%s, run_id=%s, data_length=%d", sseEvent.EventType, runID, len(sseEvent.Data))

				// Handle different event types
				switch sseEvent.EventType {
				case "metric_update":
					// Forward metric_update events directly to frontend
					// Format: {"event": "metric_update", "data": {...metric data...}}
					forwardEvent := map[string]interface{}{
						"event":  "metric_update",
						"run_id": runID,
					}

					// Parse the metric data
					var metricData map[string]interface{}
					if err := json.Unmarshal(sseEvent.Data, &metricData); err == nil {
						forwardEvent["data"] = metricData
					} else {
						// If parsing fails, forward raw data
						forwardEvent["raw_data"] = string(sseEvent.Data)
						log.Printf("Warning: Failed to parse metric data for run_id=%s: %v", runID, err)
					}

					eventJSON, err := json.Marshal(forwardEvent)
					if err != nil {
						log.Printf("Failed to marshal metric_update event for run_id=%s: %v", runID, err)
						continue
					}

					// Publish to Redis Pub/Sub
					if err := h.redisClient.Publish(ctx, eventChannel, eventJSON).Err(); err != nil {
						log.Printf("Failed to publish metric_update to Redis, run_id=%s: %v", runID, err)
						continue
					}

					// Reduced logging - only log periodically to avoid spam
					// Uncomment for full debugging:
					// log.Printf("Forwarded metric_update event to Redis for run_id=%s", runID)

				case "status_change":
					// Status changes are handled by callback, but we can forward them too
					forwardEvent := map[string]interface{}{
						"event":  sseEvent.EventType,
						"run_id": runID,
					}

					var statusData map[string]interface{}
					if err := json.Unmarshal(sseEvent.Data, &statusData); err == nil {
						forwardEvent["data"] = statusData
					}

					eventJSON, _ := json.Marshal(forwardEvent)
					h.redisClient.Publish(ctx, eventChannel, eventJSON)

				case "complete":
					// Simulation completed - forward event and stop proxy
					forwardEvent := map[string]interface{}{
						"event":  sseEvent.EventType,
						"run_id": runID,
					}

					var statusData map[string]interface{}
					if err := json.Unmarshal(sseEvent.Data, &statusData); err == nil {
						forwardEvent["data"] = statusData
					}

					eventJSON, _ := json.Marshal(forwardEvent)
					h.redisClient.Publish(ctx, eventChannel, eventJSON)

					// Stop the metrics proxy after forwarding completion event
					log.Printf("Received complete event for run_id=%s, stopping metrics proxy", runID)
					cancel() // Cancel context to stop the proxy
					return

				case "error":
					// Forward error events
					forwardEvent := map[string]interface{}{
						"event":  "error",
						"run_id": runID,
					}

					var errorData map[string]interface{}
					if err := json.Unmarshal(sseEvent.Data, &errorData); err == nil {
						forwardEvent["data"] = errorData
					}

					eventJSON, _ := json.Marshal(forwardEvent)
					h.redisClient.Publish(ctx, eventChannel, eventJSON)

				default:
					// Forward unknown events as-is
					log.Printf("Received unknown event type '%s' for run_id=%s", sseEvent.EventType, runID)
				}
			}
		}
	}()
}
