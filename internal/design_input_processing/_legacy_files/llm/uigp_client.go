package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

type UIGPClient struct {
	BaseURL string
	HTTP    *http.Client
}

func NewUIGP() *UIGPClient {
	base := os.Getenv("UIGP_BASE_URL")
	if base == "" {
		base = "http://localhost:8088"
	}
	return &UIGPClient{
		BaseURL: base,
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	SpecSummary json.RawMessage `json:"spec_summary"`
	History     []ChatMessage   `json:"history"`
	Message     string          `json:"message"`
	Mode        string          `json:"mode,omitempty"`
}

type ChatResponse struct {
	OK      bool           `json:"ok"`
	Answer  string         `json:"answer"`
	Source  string         `json:"source,omitempty"`
	Refs    []string       `json:"refs,omitempty"`
	Signals map[string]int `json:"signals,omitempty"`
}

func (c *UIGPClient) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	b, _ := json.Marshal(req)

	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat", bytes.NewReader(b))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("uigp chat: %w", err)
	}
	defer resp.Body.Close()

	var out ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("uigp decode: %w", err)
	}
	if resp.StatusCode >= 400 || !out.OK {
		return &out, fmt.Errorf("uigp error (status %d)", resp.StatusCode)
	}
	return &out, nil
}
