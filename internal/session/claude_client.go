package session

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
)

const (
	claudeWebBaseURL = "https://claude.ai/api"
)

// ClaudeWebClient talks to the Claude.ai web API using a session cookie.
type ClaudeWebClient struct {
	session    *Session
	httpClient *http.Client
	orgID      string
}

func NewClaudeWebClient(s *Session) *ClaudeWebClient {
	return &ClaudeWebClient{
		session: s,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// ClaudeWebRequest is the request format for Claude.ai web API.
type ClaudeWebRequest struct {
	Prompt            string        `json:"prompt"`
	Timezone          string        `json:"timezone"`
	Model             string        `json:"model,omitempty"`
	Attachments       []interface{} `json:"attachments"`
	Files             []interface{} `json:"files"`
	ConversationID    string        `json:"conversation_uuid,omitempty"`
	ParentMessageUUID string        `json:"parent_message_uuid,omitempty"`
}

// ClaudeWebMessage is a message in the web API format.
type ClaudeWebMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// SendMessage sends a message and returns a streaming response.
// Returns an io.ReadCloser that emits SSE events from Claude.ai.
func (c *ClaudeWebClient) SendMessage(ctx context.Context, orgID, conversationID string, req ClaudeWebRequest) (*http.Response, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/organizations/%s/chat_conversations/%s/completion",
		claudeWebBaseURL, orgID, conversationID)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}

	return resp, nil
}

// GetOrCreateConversation creates or retrieves a conversation ID.
func (c *ClaudeWebClient) CreateConversation(ctx context.Context, orgID string) (string, error) {
	type convReq struct {
		Name string `json:"name"`
	}
	type convResp struct {
		UUID string `json:"uuid"`
	}

	data, _ := json.Marshal(convReq{Name: ""})
	url := fmt.Sprintf("%s/organizations/%s/chat_conversations", claudeWebBaseURL, orgID)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	c.setHeaders(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("create conversation: HTTP %d", resp.StatusCode)
	}

	var cr convResp
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return "", err
	}
	return cr.UUID, nil
}

// GetOrganization fetches the primary organization ID for this session.
func (c *ClaudeWebClient) GetOrganization(ctx context.Context) (string, error) {
	type org struct {
		UUID string `json:"uuid"`
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		claudeWebBaseURL+"/organizations", nil)
	if err != nil {
		return "", err
	}
	c.setHeaders(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("get organizations: HTTP %d", resp.StatusCode)
	}

	var orgs []org
	if err := json.NewDecoder(resp.Body).Decode(&orgs); err != nil {
		return "", err
	}
	if len(orgs) == 0 {
		return "", fmt.Errorf("no organizations found")
	}
	return orgs[0].UUID, nil
}

func (c *ClaudeWebClient) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream,application/json")
	req.Header.Set("Cookie", "sessionKey="+c.session.SessionKey)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://claude.ai/")
	req.Header.Set("Origin", "https://claude.ai")
}

// SSEEvent represents a parsed SSE event.
type SSEEvent struct {
	Event string
	Data  string
}

// ParseSSEStream reads SSE events from a reader.
func ParseSSEStream(r io.Reader) <-chan SSEEvent {
	ch := make(chan SSEEvent, 16)
	go func() {
		defer close(ch)
		scanner := bufio.NewScanner(r)
		var event SSEEvent
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				if event.Data != "" {
					ch <- event
				}
				event = SSEEvent{}
				continue
			}
			if strings.HasPrefix(line, "event:") {
				event.Event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			} else if strings.HasPrefix(line, "data:") {
				event.Data = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			}
		}
	}()
	return ch
}
