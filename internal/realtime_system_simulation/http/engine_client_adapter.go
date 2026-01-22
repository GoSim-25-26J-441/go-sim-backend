package http

import (
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/service"
)

// EngineClientAdapter adapts the HTTP client to the service EngineClient interface
type EngineClientAdapter struct {
	httpClient *SimulationEngineClient
}

// NewEngineClientAdapter creates a new adapter
func NewEngineClientAdapter(httpClient *SimulationEngineClient) *EngineClientAdapter {
	return &EngineClientAdapter{httpClient: httpClient}
}

// GetRunSummary fetches summary from engine and converts to service type
func (a *EngineClientAdapter) GetRunSummary(engineRunID string) (*service.RunSummaryResponse, error) {
	httpResp, err := a.httpClient.GetRunSummary(engineRunID)
	if err != nil {
		return nil, err
	}

	// Convert http response to service response
	resp := &service.RunSummaryResponse{
		Summary: struct {
			RunID            string                 `json:"run_id"`
			TotalRequests    int64                  `json:"total_requests,omitempty"`
			TotalErrors      int64                  `json:"total_errors,omitempty"`
			TotalDurationMs  int64                  `json:"total_duration_ms,omitempty"`
			Metrics          map[string]interface{} `json:"metrics,omitempty"`
			SummaryData      map[string]interface{} `json:"summary_data,omitempty"`
			CreatedAtUnixMs  int64                  `json:"created_at_unix_ms,omitempty"`
			StartedAtUnixMs  int64                  `json:"started_at_unix_ms,omitempty"`
			EndedAtUnixMs    int64                  `json:"ended_at_unix_ms,omitempty"`
		}{
			RunID:            httpResp.Summary.RunID,
			TotalRequests:    httpResp.Summary.TotalRequests,
			TotalErrors:      httpResp.Summary.TotalErrors,
			TotalDurationMs:  httpResp.Summary.TotalDurationMs,
			Metrics:          httpResp.Summary.Metrics,
			SummaryData:      httpResp.Summary.SummaryData,
			CreatedAtUnixMs:  httpResp.Summary.CreatedAtUnixMs,
			StartedAtUnixMs:  httpResp.Summary.StartedAtUnixMs,
			EndedAtUnixMs:    httpResp.Summary.EndedAtUnixMs,
		},
	}

	return resp, nil
}

// GetRunMetrics fetches metrics from engine and converts to service type
func (a *EngineClientAdapter) GetRunMetrics(engineRunID string) ([]service.MetricDataPointResponse, error) {
	httpMetrics, err := a.httpClient.GetRunMetrics(engineRunID)
	if err != nil {
		return nil, err
	}

	// Convert http metrics to service metrics
	metrics := make([]service.MetricDataPointResponse, len(httpMetrics))
	for i, httpMetric := range httpMetrics {
		metrics[i] = service.MetricDataPointResponse{
			TimestampMs: httpMetric.TimestampMs,
			MetricType:  httpMetric.MetricType,
			MetricValue: httpMetric.MetricValue,
			ServiceID:   httpMetric.ServiceID,
			NodeID:      httpMetric.NodeID,
			Tags:        httpMetric.Tags,
		}
	}

	return metrics, nil
}
