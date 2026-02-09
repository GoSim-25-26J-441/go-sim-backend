package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/graph"
)

// GraphService handles graph-related operations
type GraphService struct {
	upstreamClient *UpstreamClient
}

// NewGraphService creates a new graph service
func NewGraphService(upstreamClient *UpstreamClient) *GraphService {
	return &GraphService{
		upstreamClient: upstreamClient,
	}
}

// GetGraphvizDOT generates GraphViz DOT format from a job's export
func (s *GraphService) GetGraphvizDOT(ctx context.Context, jobID string) ([]byte, error) {
	recordGraphServiceCall()
	logger := NewLogger(ctx)
	logger.LogInfof("get_graphviz_dot", "generating DOT for job_id=%s", jobID)

	// Fetch export as JSON
	specMap, err := s.upstreamClient.GetExportJSON(ctx, jobID)
	if err != nil {
		logger.LogError("get_graphviz_dot", err)
		return nil, fmt.Errorf("get export: %w", err)
	}

	// Convert map to graph.Architecture
	var arch graph.Architecture
	specBytes, err := json.Marshal(specMap)
	if err != nil {
		logger.LogError("get_graphviz_dot", err)
		return nil, fmt.Errorf("marshal spec: %w", err)
	}
	if err := json.Unmarshal(specBytes, &arch); err != nil {
		logger.LogError("get_graphviz_dot", err)
		return nil, fmt.Errorf("unmarshal spec: %w", err)
	}

	// Convert Architecture to Graph
	g, err := graph.FromSpec(arch)
	if err != nil {
		logger.LogError("get_graphviz_dot", err)
		return nil, fmt.Errorf("build graph: %w", err)
	}

	// Convert Graph to DOT
	dotBytes := graph.ToDOT(g)
	logger.LogInfof("get_graphviz_dot", "generated DOT (%d bytes) for job_id=%s", len(dotBytes), jobID)
	return dotBytes, nil
}
