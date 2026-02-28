package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// LLMClient handles communication with the LLM service
type LLMClient struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

// NewLLMClient creates a new LLM client
func NewLLMClient(baseURL, apiKey string) *LLMClient {
	return &LLMClient{
		BaseURL: baseURL,
		APIKey:  apiKey,
		HTTP:    &http.Client{Timeout: 180 * time.Second},
	}
}

// ChatMessage represents a message in the chat history
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AttachmentRequest represents an attachment in the API request
type AttachmentRequest struct {
	Name        string `json:"name"`
	ContentType string `json:"content_type,omitempty"`
	SizeBytes   *int64 `json:"size_bytes,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
	Kind        string `json:"kind,omitempty"`
}

// ChatRequest represents the request to the LLM service
type ChatRequest struct {
	Message     string              `json:"message"`
	History     []ChatMessage       `json:"history,omitempty"`
	Mode        string              `json:"mode,omitempty"`
	Detail      string              `json:"detail,omitempty"`
	DiagramJSON json.RawMessage     `json:"diagram_json,omitempty"`
	SpecSummary json.RawMessage     `json:"spec_summary,omitempty"`
	Attachments []AttachmentRequest `json:"attachments,omitempty"`
}

// SourceInfo represents the source information in the response
type SourceInfo struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// MetaInfo represents metadata in the response
type MetaInfo struct {
	ContextUsed string `json:"context_used,omitempty"`
	HistoryUsed *int   `json:"history_used,omitempty"`
	LatencyMs   *int   `json:"latency_ms,omitempty"`
	ModeInvalid bool   `json:"mode_invalid,omitempty"`
	ModeUsed    string `json:"mode_used,omitempty"`
	Blocked     bool   `json:"blocked,omitempty"`
}

// ChatResponse represents the response from the LLM service
type ChatResponse struct {
	OK      bool                   `json:"ok"`
	Answer  string                 `json:"answer"`
	Source  SourceInfo             `json:"source"`
	Refs    []string               `json:"refs,omitempty"`
	Signals map[string]interface{} `json:"signals,omitempty"`
	Meta    MetaInfo               `json:"meta,omitempty"`
}

// Chat sends a chat request to the LLM service
func (c *LLMClient) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Console log: what we pass to UIGP (kept for debugging)
	fmt.Printf("\n[UIGP] URL: %s/api/v1/chat\n", c.BaseURL)
	fmt.Printf("[UIGP] Headers: Content-Type=application/json, X-API-Key=%s\n", c.APIKey)
	fmt.Printf("[UIGP] Body: %s\n\n", string(body))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/v1/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		httpReq.Header.Set("X-API-Key", c.APIKey)
	}

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("llm request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("llm error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if !chatResp.OK {
		return &chatResp, fmt.Errorf("llm returned ok=false")
	}

	return &chatResp, nil
}
