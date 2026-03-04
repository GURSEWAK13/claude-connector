package session

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	anthropicBaseURL     = "https://api.anthropic.com/v1"
	anthropicAPIVersion  = "2023-06-01"
)

// AnthropicClient calls the official Anthropic API using an API key.
type AnthropicClient struct {
	session    *Session
	httpClient *http.Client
}

func NewAnthropicClient(s *Session) *AnthropicClient {
	return &AnthropicClient{
		session: s,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// SendMessages forwards an Anthropic API /v1/messages request.
// body should be the raw JSON request body.
// Returns the raw response so the proxy can forward it as-is.
func (c *AnthropicClient) SendMessages(ctx context.Context, body []byte) (*http.Response, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		anthropicBaseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-API-Key", c.session.SessionKey)
	httpReq.Header.Set("Anthropic-Version", anthropicAPIVersion)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}

	return resp, nil
}

// AnthropicRequest mirrors the Anthropic API /v1/messages request.
type AnthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []AnthropicMessage `json:"messages"`
	System    string             `json:"system,omitempty"`
	Stream    bool               `json:"stream,omitempty"`
}

// AnthropicMessage is a single message in the Anthropic API format.
type AnthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []ContentBlock
}

// ContentBlock is a typed content block in the Anthropic API format.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// AnthropicResponse mirrors the Anthropic API response.
type AnthropicResponse struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"`
	Role         string    `json:"role"`
	Content      []ContentBlock `json:"content"`
	Model        string    `json:"model"`
	StopReason   string    `json:"stop_reason"`
	StopSequence *string   `json:"stop_sequence"`
	Usage        struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// AnthropicStreamEvent is a single SSE event in the streaming API.
type AnthropicStreamEvent struct {
	Type  string          `json:"type"`
	Index int             `json:"index,omitempty"`
	Delta *ContentDelta   `json:"delta,omitempty"`
	Error *StreamError    `json:"error,omitempty"`
}

type ContentDelta struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type StreamError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// ParseAnthropicRequest decodes raw JSON into an AnthropicRequest.
func ParseAnthropicRequest(body []byte) (*AnthropicRequest, error) {
	var req AnthropicRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("parse anthropic request: %w", err)
	}
	return &req, nil
}

// DrainAndClose reads and discards the response body then closes it.
func DrainAndClose(r io.ReadCloser) {
	if r != nil {
		_, _ = io.Copy(io.Discard, r)
		_ = r.Close()
	}
}
