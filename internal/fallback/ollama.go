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
	"sync/atomic"
	"time"

	"github.com/GURSEWAK13/claude-connector/internal/config"
	"github.com/GURSEWAK13/claude-connector/internal/session"
)

type OllamaBackend struct {
	cfg       *config.Config
	available int32 // atomic bool
}

func NewOllamaBackend(cfg *config.Config) *OllamaBackend {
	return &OllamaBackend{cfg: cfg}
}

func (o *OllamaBackend) Name() string        { return "Ollama" }
func (o *OllamaBackend) DefaultModel() string { return o.cfg.Fallback.OllamaDefaultModel }

func (o *OllamaBackend) IsAvailable() bool {
	ok := probeURL(o.cfg.Fallback.OllamaURL + "/api/tags")
	if ok {
		atomic.StoreInt32(&o.available, 1)
	} else {
		atomic.StoreInt32(&o.available, 0)
	}
	return ok
}

// ollamaRequest is the Ollama /api/chat request.
type ollamaRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Options  map[string]any  `json:"options,omitempty"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ollamaChunk is a single streaming response chunk from Ollama.
type ollamaChunk struct {
	Model     string        `json:"model"`
	CreatedAt time.Time     `json:"created_at"`
	Message   ollamaMessage `json:"message"`
	Done      bool          `json:"done"`
}

func (o *OllamaBackend) Forward(ctx context.Context, w http.ResponseWriter, body []byte, req *session.AnthropicRequest) error {
	model := req.Model
	if model == "" || !strings.Contains(model, ":") {
		model = o.cfg.Fallback.OllamaDefaultModel
	}

	// Convert Anthropic messages to Ollama messages
	var messages []ollamaMessage
	if req.System != "" {
		messages = append(messages, ollamaMessage{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		content := extractContent(m.Content)
		messages = append(messages, ollamaMessage{Role: m.Role, Content: content})
	}

	ollamaReq := ollamaRequest{
		Model:    model,
		Messages: messages,
		Stream:   true,
	}

	data, err := json.Marshal(ollamaReq)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		o.cfg.Fallback.OllamaURL+"/api/chat", bytes.NewReader(data))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama error: HTTP %d", resp.StatusCode)
	}

	if req.Stream {
		return o.streamResponse(ctx, w, resp.Body, model)
	}
	return o.collectResponse(w, resp.Body, model)
}

func (o *OllamaBackend) streamResponse(ctx context.Context, w http.ResponseWriter, body io.Reader, model string) error {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)

	// Write message_start
	msgID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
	startEvent := fmt.Sprintf(`{"type":"message_start","message":{"id":%q,"type":"message","role":"assistant","content":[],"model":%q,"stop_reason":null,"usage":{"input_tokens":1,"output_tokens":1}}}`, msgID, model)
	fmt.Fprintf(w, "event: message_start\ndata: %s\n\n", startEvent)
	fmt.Fprintf(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
	fmt.Fprintf(w, "event: ping\ndata: {\"type\":\"ping\"}\n\n")
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

		var chunk ollamaChunk
		if err := json.Unmarshal(scanner.Bytes(), &chunk); err != nil {
			continue
		}

		if chunk.Message.Content != "" {
			delta := fmt.Sprintf(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":%s}}`,
				jsonString(chunk.Message.Content))
			fmt.Fprintf(w, "event: content_block_delta\ndata: %s\n\n", delta)
			if flusher != nil {
				flusher.Flush()
			}
		}

		if chunk.Done {
			break
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

func (o *OllamaBackend) collectResponse(w http.ResponseWriter, body io.Reader, model string) error {
	var fullText strings.Builder
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		var chunk ollamaChunk
		if err := json.Unmarshal(scanner.Bytes(), &chunk); err != nil {
			continue
		}
		fullText.WriteString(chunk.Message.Content)
	}

	msgID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
	resp := map[string]any{
		"id":          msgID,
		"type":        "message",
		"role":        "assistant",
		"content":     []map[string]any{{"type": "text", "text": fullText.String()}},
		"model":       model,
		"stop_reason": "end_turn",
		"usage":       map[string]any{"input_tokens": 1, "output_tokens": 1},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(resp)
}

func extractContent(content any) string {
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
	}
	return fmt.Sprintf("%v", content)
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// Ensure OllamaBackend implements Backend
var _ Backend = (*OllamaBackend)(nil)
