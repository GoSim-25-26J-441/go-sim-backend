package http

import (
	"context"
	"time"

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

// GetRunSummary fetches export from engine and builds a summary response (engine has no separate summary endpoint)
func (a *EngineClientAdapter) GetRunSummary(engineRunID string) (*service.RunSummaryResponse, error) {
	exportResp, err := a.httpClient.ExportRun(engineRunID)
	if err != nil {
		return nil, err
	}

	var totalDurationMs int64
	if exportResp.Run != nil {
		totalDurationMs = exportResp.Run.SimulationDurationMs
		if totalDurationMs == 0 {
			totalDurationMs = exportResp.Run.RealDurationMs
		}
	}
	if totalDurationMs == 0 && exportResp.Input.DurationMs > 0 {
		totalDurationMs = exportResp.Input.DurationMs
	}

	metricsMap := make(map[string]interface{})
	if exportResp.Metrics != nil {
		for k, v := range exportResp.Metrics {
			metricsMap[k] = v
		}
	}

	var finalConfig map[string]interface{}
	if exportResp.FinalConfig != nil {
		finalConfig = make(map[string]interface{}, len(exportResp.FinalConfig))
		for k, v := range exportResp.FinalConfig {
			finalConfig[k] = v
		}
	}

	return &service.RunSummaryResponse{
		Summary: struct {
			RunID            string                 `json:"run_id"`
			TotalRequests    int64                  `json:"total_requests,omitempty"`
			TotalErrors      int64                  `json:"total_errors,omitempty"`
			TotalDurationMs  int64                  `json:"total_duration_ms,omitempty"`
			Metrics          map[string]interface{} `json:"metrics,omitempty"`
			SummaryData      map[string]interface{} `json:"summary_data,omitempty"`
			FinalConfig      map[string]interface{} `json:"final_config,omitempty"`
			CreatedAtUnixMs  int64                  `json:"created_at_unix_ms,omitempty"`
			StartedAtUnixMs  int64                  `json:"started_at_unix_ms,omitempty"`
			EndedAtUnixMs    int64                  `json:"ended_at_unix_ms,omitempty"`
		}{
			RunID:           engineRunID,
			TotalDurationMs: totalDurationMs,
			Metrics:         metricsMap,
			SummaryData:     metricsMap,
			FinalConfig:     finalConfig,
		},
	}, nil
}

// GetRunMetrics fetches time-series from engine and converts to service type (slice of data points)
func (a *EngineClientAdapter) GetRunMetrics(engineRunID string) ([]service.MetricDataPointResponse, error) {
	ctx := context.Background()
	tsResp, err := a.httpClient.GetRunMetricsTimeSeries(ctx, engineRunID, nil)
	if err != nil {
		return nil, err
	}
	if tsResp == nil || len(tsResp.Points) == 0 {
		return nil, nil
	}

	metrics := make([]service.MetricDataPointResponse, 0, len(tsResp.Points))
	for _, p := range tsResp.Points {
		tsMs := int64(0)
		if t, err := time.Parse(time.RFC3339, p.Timestamp); err == nil {
			tsMs = t.UnixMilli()
		}
		tags := make(map[string]interface{})
		if p.Labels != nil {
			for k, v := range p.Labels {
				tags[k] = v
			}
		}
		dp := service.MetricDataPointResponse{
			TimestampMs: tsMs,
			MetricType:  p.Metric,
			MetricValue: p.Value,
			ServiceID:   p.Labels["service_id"],
			NodeID:      p.Labels["node_id"],
			Tags:        tags,
		}
		if dp.ServiceID == "" {
			dp.ServiceID = p.Labels["service"]
		}
		metrics = append(metrics, dp)
	}
	return metrics, nil
}
