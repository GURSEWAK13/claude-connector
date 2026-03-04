package fallback

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/GURSEWAK13/claude-connector/internal/config"
	"github.com/GURSEWAK13/claude-connector/internal/session"
)

// LMStudioBackend forwards requests to a local LM Studio server.
// LM Studio exposes an OpenAI-compatible API.
type LMStudioBackend struct {
	cfg *config.Config
}

func NewLMStudioBackend(cfg *config.Config) *LMStudioBackend {
	return &LMStudioBackend{cfg: cfg}
}

func (l *LMStudioBackend) Name() string         { return "LM Studio" }
func (l *LMStudioBackend) DefaultModel() string { return "local-model" }

func (l *LMStudioBackend) IsAvailable() bool {
	return probeURL(l.cfg.Fallback.LMStudioURL + "/v1/models")
}

// openAIChatRequest mirrors the OpenAI chat completions request (LM Studio is OpenAI-compatible).
type openAIChatRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Stream      bool            `json:"stream"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

func (l *LMStudioBackend) Forward(ctx context.Context, w http.ResponseWriter, body []byte, req *session.AnthropicRequest) error {
	// Convert to OpenAI format
	var messages []openAIMessage
	if req.System != "" {
		messages = append(messages, openAIMessage{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		messages = append(messages, openAIMessage{
			Role:    m.Role,
			Content: extractContent(m.Content),
		})
	}

	oaiReq := openAIChatRequest{
		Model:     l.DefaultModel(),
		Messages:  messages,
		Stream:    true,
		MaxTokens: req.MaxTokens,
	}

	data, err := json.Marshal(oaiReq)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		l.cfg.Fallback.LMStudioURL+"/v1/chat/completions", bytes.NewReader(data))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("lmstudio request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("lmstudio error: HTTP %d", resp.StatusCode)
	}

	if req.Stream {
		return l.streamResponse(ctx, w, resp.Body, req.Model)
	}
	return l.collectResponse(w, resp.Body, req.Model)
}

func (l *LMStudioBackend) streamResponse(ctx context.Context, w http.ResponseWriter, body io.Reader, model string) error {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)

	msgID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
	fmt.Fprintf(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":%q,\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":%q,\"stop_reason\":null,\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n", msgID, model)
	fmt.Fprintf(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
	if flusher != nil {
		flusher.Flush()
	}

	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk openAIChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				deltaEvent := fmt.Sprintf(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":%s}}`,
					jsonString(choice.Delta.Content))
				fmt.Fprintf(w, "event: content_block_delta\ndata: %s\n\n", deltaEvent)
				if flusher != nil {
					flusher.Flush()
				}
			}
		}
	}

	fmt.Fprintf(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
	fmt.Fprintf(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\",\"stop_sequence\":null},\"usage\":{\"output_tokens\":1}}\n\n")
	fmt.Fprintf(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	if flusher != nil {
		flusher.Flush()
	}

	return scanner.Err()
}

func (l *LMStudioBackend) collectResponse(w http.ResponseWriter, body io.Reader, model string) error {
	respBody, err := io.ReadAll(body)
	if err != nil {
		return err
	}

	// OpenAI format -> Anthropic format
	var oaiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &oaiResp); err != nil {
		return err
	}

	text := ""
	if len(oaiResp.Choices) > 0 {
		text = oaiResp.Choices[0].Message.Content
	}

	msgID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
	resp := map[string]any{
		"id":          msgID,
		"type":        "message",
		"role":        "assistant",
		"content":     []map[string]any{{"type": "text", "text": text}},
		"model":       model,
		"stop_reason": "end_turn",
		"usage":       map[string]any{"input_tokens": 1, "output_tokens": 1},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(resp)
}

// Ensure LMStudioBackend implements Backend
var _ Backend = (*LMStudioBackend)(nil)
