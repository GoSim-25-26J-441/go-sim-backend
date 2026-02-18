package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// UpstreamClient handles communication with the upstream LLM service
type UpstreamClient struct {
	baseURL       string
	defaultClient *http.Client
	longClient    *http.Client // For operations that need longer timeouts (90s)
	fuseClient    *http.Client // For fuse operations (3min)
}

// NewUpstreamClient creates a new upstream client
func NewUpstreamClient(baseURL string) *UpstreamClient {
	return &UpstreamClient{
		baseURL: baseURL,
		defaultClient: &http.Client{
			Timeout: DefaultTimeout,
		},
		longClient: &http.Client{
			Timeout: LongTimeout,
		},
		fuseClient: &http.Client{
			Timeout: FuseTimeout,
		},
	}
}

// GetIntermediate fetches the intermediate graph for a job
func (c *UpstreamClient) GetIntermediate(ctx context.Context, jobID string) (*http.Response, error) {
	logger := NewLogger(ctx)
	start := time.Now()
	reqURL := c.baseURL + "/jobs/" + jobID + "/intermediate"
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		logger.LogError("get_intermediate", err)
		recordUpstreamCall(time.Since(start), err)
		return nil, fmt.Errorf("create request: %w", err)
	}
	resp, err := c.defaultClient.Do(req)
	duration := time.Since(start)
	if err != nil {
		logger.LogError("get_intermediate", err)
		recordUpstreamCall(duration, err)
		return nil, fmt.Errorf("upstream request failed: %w", err)
	}
	if resp.StatusCode >= 400 {
		logger.LogWarnf("get_intermediate", "upstream returned status %d", resp.StatusCode)
		recordUpstreamCall(duration, fmt.Errorf("status %d", resp.StatusCode))
	} else {
		recordUpstreamCall(duration, nil)
	}
	return resp, nil
}

// Fuse triggers fusion for a job
func (c *UpstreamClient) Fuse(ctx context.Context, jobID, userID string) (*http.Response, error) {
	logger := NewLogger(ctx)
	reqURL := c.baseURL + "/jobs/" + jobID + "/fuse"
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, nil)
	if err != nil {
		logger.LogError("fuse", err)
		return nil, fmt.Errorf("create request: %w", err)
	}
	if userID != "" {
		req.Header.Set("X-User-Id", userID)
	}
	logger.LogInfof("fuse", "triggering fuse for job_id=%s user_id=%s", jobID, userID)
	resp, err := c.fuseClient.Do(req)
	if err != nil {
		logger.LogError("fuse", err)
		return nil, fmt.Errorf("upstream request failed: %w", err)
	}
	if resp.StatusCode >= 400 {
		logger.LogWarnf("fuse", "upstream returned status %d", resp.StatusCode)
	}
	return resp, nil
}

// ExportOptions contains options for export requests
type ExportOptions struct {
	Format   string
	Download string
}

// Export fetches the export for a job
func (c *UpstreamClient) Export(ctx context.Context, jobID string, opts ExportOptions) (*http.Response, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse base URL: %w", err)
	}
	u.Path = u.Path + "/jobs/" + jobID + "/export"

	q := u.Query()
	if opts.Format == "" {
		opts.Format = "json"
	}
	q.Set("format", opts.Format)
	if opts.Download == "" {
		opts.Download = "false"
	}
	q.Set("download", opts.Download)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	return c.longClient.Do(req)
}

// GetExportJSON fetches and parses the export as JSON
func (c *UpstreamClient) GetExportJSON(ctx context.Context, jobID string) (map[string]any, error) {
	resp, err := c.Export(ctx, jobID, ExportOptions{Format: "json", Download: "false"})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("export failed with status %d", resp.StatusCode)
	}

	var spec map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&spec); err != nil {
		return nil, fmt.Errorf("decode JSON: %w", err)
	}
	return spec, nil
}

// GetReport fetches the report for a job
func (c *UpstreamClient) GetReport(ctx context.Context, jobID string, query string) (*http.Response, error) {
	reqURL := c.baseURL + "/jobs/" + jobID + "/report"
	if query != "" {
		reqURL += "?" + query
	}
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	return c.defaultClient.Do(req)
}

// GetReportJSON fetches and parses the report as JSON
func (c *UpstreamClient) GetReportJSON(ctx context.Context, jobID string) (map[string]any, error) {
	resp, err := c.GetReport(ctx, jobID, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("report failed with status %d", resp.StatusCode)
	}

	var report map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		return nil, fmt.Errorf("decode JSON: %w", err)
	}
	return report, nil
}

// GetIntermediateJSON fetches and parses the intermediate graph as JSON
func (c *UpstreamClient) GetIntermediateJSON(ctx context.Context, jobID string) (map[string]any, error) {
	resp, err := c.GetIntermediate(ctx, jobID)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("intermediate failed with status %d", resp.StatusCode)
	}

	var ig map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&ig); err != nil {
		return nil, fmt.Errorf("decode JSON: %w", err)
	}
	return ig, nil
}

// Ingest forwards an ingest request to the upstream service
func (c *UpstreamClient) Ingest(ctx context.Context, body io.Reader, headers http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/ingest", body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if headers != nil {
		req.Header = headers.Clone()
	}
	return c.longClient.Do(req)
}
