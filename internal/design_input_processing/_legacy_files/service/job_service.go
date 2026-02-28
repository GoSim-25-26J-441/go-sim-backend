package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// JobService handles job-related business logic
type JobService struct {
	upstreamClient *UpstreamClient
}

// NewJobService creates a new job service
func NewJobService(upstreamClient *UpstreamClient) *JobService {
	return &JobService{
		upstreamClient: upstreamClient,
	}
}

// JobSummary represents a summary of a job
type JobSummary struct {
	ID           string `json:"id"`
	Services     int    `json:"services"`
	Dependencies int    `json:"dependencies"`
	Gaps         int    `json:"gaps"`
}

// ListJobIDs returns all job IDs for a given user
func (s *JobService) ListJobIDs(userID string) ([]string, error) {
	dir := chatBaseDir(userID)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read directory: %w", err)
	}

	var jobIDs []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "chat-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		id := strings.TrimSuffix(strings.TrimPrefix(name, "chat-"), ".jsonl")
		jobIDs = append(jobIDs, id)
	}

	return jobIDs, nil
}

// GetJobSummaries returns summaries for all jobs for a user
func (s *JobService) GetJobSummaries(ctx context.Context, userID string) ([]JobSummary, error) {
	recordJobServiceCall()
	logger := NewLogger(ctx)
	ids, err := s.ListJobIDs(userID)
	if err != nil {
		logger.LogError("get_job_summaries", err)
		return nil, fmt.Errorf("list job IDs: %w", err)
	}

	logger.LogInfof("get_job_summaries", "processing %d jobs for user_id=%s", len(ids), userID)
	out := make([]JobSummary, 0, len(ids))

	for _, id := range ids {
		ig, _ := s.upstreamClient.GetIntermediateJSON(ctx, id)
		report, _ := s.upstreamClient.GetReportJSON(ctx, id)

		out = append(out, s.SummarizeJob(id, ig, report))
	}

	return out, nil
}

// SummarizeJob creates a summary from intermediate graph and report data
func (s *JobService) SummarizeJob(id string, ig, report map[string]any) JobSummary {
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

// chatBaseDir returns the base directory for chat logs for a given user
func chatBaseDir(userID string) string {
	dir := os.Getenv("CHAT_LOG_DIR")
	if dir == "" {
		dir = filepath.FromSlash("internal/design_input_processing/data/chat_logs")
	}

	// userID should always be provided from Firebase auth context
	// Sanitize to prevent path traversal
	userID = strings.ReplaceAll(userID, "..", "_")
	userID = strings.ReplaceAll(userID, "/", "_")
	userID = strings.ReplaceAll(userID, "\\", "_")

	return filepath.Join(dir, userID)
}
