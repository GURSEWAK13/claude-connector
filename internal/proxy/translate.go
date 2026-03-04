package proxy

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/GURSEWAK13/claude-connector/internal/session"
)

// AnthropicToClaudeWebRequest converts an Anthropic API request to Claude.ai web format.
// It extracts the text from all user messages and builds a single prompt string.
func AnthropicToClaudeWebRequest(req *session.AnthropicRequest) session.ClaudeWebRequest {
	var parts []string

	// Add system prompt if present
	if req.System != "" {
		parts = append(parts, "System: "+req.System)
	}

	// Build conversation history as a prompt
	for _, msg := range req.Messages {
		text := extractText(msg.Content)
		switch msg.Role {
		case "user":
			parts = append(parts, "Human: "+text)
		case "assistant":
			parts = append(parts, "Assistant: "+text)
		}
	}

	// Map model name (Anthropic → Claude.ai web model slug)
	model := mapModel(req.Model)

	return session.ClaudeWebRequest{
		Prompt:      strings.Join(parts, "\n\n"),
		Timezone:    "UTC",
		Model:       model,
		Attachments: []interface{}{},
		Files:       []interface{}{},
	}
}

// extractText extracts plain text from an Anthropic message content (string or []ContentBlock).
func extractText(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, item := range v {
			if block, ok := item.(map[string]any); ok {
				if t, ok := block["text"].(string); ok {
					parts = append(parts, t)
				}
			}
		}
		return strings.Join(parts, "")
	case []session.ContentBlock:
		var parts []string
		for _, b := range v {
			if b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "")
	case json.RawMessage:
		// Try to unmarshal as string first
		var s string
		if err := json.Unmarshal(v, &s); err == nil {
			return s
		}
		// Try as array of blocks
		var blocks []session.ContentBlock
		if err := json.Unmarshal(v, &blocks); err == nil {
			var parts []string
			for _, b := range blocks {
				parts = append(parts, b.Text)
			}
			return strings.Join(parts, "")
		}
		return string(v)
	}
	return fmt.Sprintf("%v", content)
}

// mapModel maps an Anthropic model identifier to a Claude.ai web model slug.
func mapModel(model string) string {
	switch {
	case strings.Contains(model, "claude-3-5-sonnet"):
		return "claude-3-5-sonnet-20241022"
	case strings.Contains(model, "claude-3-5-haiku"):
		return "claude-3-5-haiku-20241022"
	case strings.Contains(model, "claude-3-opus"):
		return "claude-3-opus-20240229"
	case strings.Contains(model, "claude-3-sonnet"):
		return "claude-3-sonnet-20240229"
	case strings.Contains(model, "claude-3-haiku"):
		return "claude-3-haiku-20240307"
	case strings.Contains(model, "claude-2"):
		return "claude-2.1"
	case model == "":
		return "claude-3-5-sonnet-20241022"
	default:
		return model
	}
}

// BuildAnthropicResponse builds a non-streaming Anthropic API response from accumulated text.
func BuildAnthropicResponse(model, text string, inputTokens, outputTokens int) *session.AnthropicResponse {
	return &session.AnthropicResponse{
		ID:   "msg_" + uuid.New().String()[:20],
		Type: "message",
		Role: "assistant",
		Content: []session.ContentBlock{
			{Type: "text", Text: text},
		},
		Model:      model,
		StopReason: "end_turn",
		Usage: struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		}{
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
		},
	}
}

// ClaudeWebEventToAnthropicSSE converts a Claude.ai SSE event to an Anthropic streaming SSE line.
// Returns empty string if the event should be skipped.
func ClaudeWebEventToAnthropicSSE(eventType, data string) []string {
	switch eventType {
	case "message_start":
		// Emit Anthropic message_start
		start := map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id":            "msg_" + uuid.New().String()[:20],
				"type":          "message",
				"role":          "assistant",
				"model":         "claude-3-5-sonnet-20241022",
				"content":       []any{},
				"stop_reason":   nil,
				"stop_sequence": nil,
				"usage": map[string]any{
					"input_tokens":  0,
					"output_tokens": 1,
				},
			},
		}
		b, _ := json.Marshal(start)
		return []string{
			"event: message_start",
			"data: " + string(b),
			"",
			"event: content_block_start",
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			"",
			"event: ping",
			`data: {"type":"ping"}`,
			"",
		}

	case "content_block_delta", "":
		// Parse the data as a Claude.ai completion chunk
		var claudeData map[string]any
		if err := json.Unmarshal([]byte(data), &claudeData); err != nil {
			return nil
		}

		text, _ := claudeData["completion"].(string)
		if text == "" {
			// Try "delta.text" format
			if delta, ok := claudeData["delta"].(map[string]any); ok {
				text, _ = delta["text"].(string)
			}
		}
		if text == "" {
			return nil
		}

		delta := map[string]any{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]any{
				"type": "text_delta",
				"text": text,
			},
		}
		b, _ := json.Marshal(delta)
		return []string{
			"event: content_block_delta",
			"data: " + string(b),
			"",
		}

	case "message_stop", "message_delta":
		stop := map[string]any{"type": "content_block_stop", "index": 0}
		b1, _ := json.Marshal(stop)

		msgDelta := map[string]any{
			"type": "message_delta",
			"delta": map[string]any{
				"stop_reason":   "end_turn",
				"stop_sequence": nil,
			},
			"usage": map[string]any{"output_tokens": 100},
		}
		b2, _ := json.Marshal(msgDelta)

		msgStop := map[string]any{"type": "message_stop"}
		b3, _ := json.Marshal(msgStop)

		return []string{
			"event: content_block_stop",
			"data: " + string(b1),
			"",
			"event: message_delta",
			"data: " + string(b2),
			"",
			"event: message_stop",
			"data: " + string(b3),
			"",
		}
	}
	return nil
}

// EstimateTokens provides a rough estimate of token count (4 chars ≈ 1 token).
func EstimateTokens(text string) int {
	return max(1, len(text)/4)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// nowUnix returns the current unix timestamp.
func nowUnix() int64 {
	return time.Now().Unix()
}
