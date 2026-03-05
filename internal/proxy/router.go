package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/GURSEWAK13/claude-connector/internal/fallback"
	"github.com/GURSEWAK13/claude-connector/internal/peer"
	"github.com/GURSEWAK13/claude-connector/internal/session"
)

// RouteResult describes how a request was routed.
type RouteResult struct {
	Via           string // "local", "peer:<id>", "ollama", "lmstudio"
	Duration      time.Duration
	Model         string
	SessionID     string // which session ultimately served the request
	FailoverCount int    // how many sessions were tried before success (0 = first try)
}

// Router implements the local→peer→fallback routing cascade.
type Router struct {
	pool     *session.Pool
	registry *peer.Registry
	fallback *fallback.Manager
}

func NewRouter(pool *session.Pool, registry *peer.Registry, fb *fallback.Manager) *Router {
	return &Router{
		pool:     pool,
		registry: registry,
		fallback: fb,
	}
}

// RouteRequest attempts to route the request through available channels.
// It writes the response directly to w and returns routing metadata.
func (r *Router) RouteRequest(ctx context.Context, w http.ResponseWriter, reqBody []byte, req *session.AnthropicRequest) (*RouteResult, error) {
	start := time.Now()

	// 1. Try all local sessions — retry on 429 with next available session
	tried := make(map[string]bool)
	for {
		sess := r.pool.AcquireExcluding(tried)
		if sess == nil {
			break
		}
		tried[sess.ID] = true
		result, err := r.executeLocal(ctx, w, sess, reqBody, req)
		if err == nil {
			result.Duration = time.Since(start)
			result.FailoverCount = len(tried) - 1
			return result, nil
		}
		// err means rate-limited or error — try next local session
	}

	// 2. Try available peers
	peers := r.registry.AvailablePeers()
	for i := range peers {
		result, err := r.executePeer(ctx, w, &peers[i], reqBody)
		if err == nil {
			result.Duration = time.Since(start)
			return result, nil
		}
	}

	// 3. Try fallback (Ollama / LM Studio)
	if result, err := r.executeFallback(ctx, w, reqBody, req); err == nil {
		result.Duration = time.Since(start)
		return result, nil
	}

	// 4. Nothing available
	http.Error(w, `{"type":"error","error":{"type":"rate_limit_error","message":"All sessions rate-limited, no peers available, no fallback configured"}}`, http.StatusTooManyRequests)
	return nil, fmt.Errorf("no route available")
}

func (r *Router) executeLocal(ctx context.Context, w http.ResponseWriter, sess *session.Session, rawBody []byte, req *session.AnthropicRequest) (*RouteResult, error) {
	result := &RouteResult{Via: "local", Model: req.Model, SessionID: sess.ID}

	defer func() {
		// session.Release() is called after response completes
	}()

	var respErr error
	switch sess.Type {
	case "anthropic_api":
		client := session.NewAnthropicClient(sess)
		resp, err := client.SendMessages(ctx, rawBody)
		if err != nil {
			sess.MarkError()
			return nil, err
		}
		defer resp.Body.Close()

		if session.IsRateLimitResponse(resp.StatusCode) {
			retryAfter := session.ParseRetryAfter(resp)
			sess.MarkRateLimited(retryAfter)
			return nil, fmt.Errorf("rate limited")
		}

		// Forward response headers and body
		for k, vs := range resp.Header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		_, respErr = io.Copy(w, resp.Body)
		sess.Release()

	case "claude_web":
		respErr = r.executeClaudeWeb(ctx, w, sess, req)
	}

	return result, respErr
}

func (r *Router) executeClaudeWeb(ctx context.Context, w http.ResponseWriter, sess *session.Session, req *session.AnthropicRequest) error {
	client := session.NewClaudeWebClient(sess)

	orgID, err := client.GetOrganization(ctx)
	if err != nil {
		sess.MarkError()
		return fmt.Errorf("get org: %w", err)
	}

	convID, err := client.CreateConversation(ctx, orgID)
	if err != nil {
		sess.MarkError()
		return fmt.Errorf("create conversation: %w", err)
	}

	webReq := AnthropicToClaudeWebRequest(req)

	resp, err := client.SendMessage(ctx, orgID, convID, webReq)
	if err != nil {
		sess.MarkError()
		return fmt.Errorf("send message: %w", err)
	}
	defer resp.Body.Close()

	if session.IsRateLimitResponse(resp.StatusCode) {
		retryAfter := session.ParseRetryAfter(resp)
		sess.MarkRateLimited(retryAfter)
		return fmt.Errorf("rate limited")
	}

	if resp.StatusCode >= 400 {
		sess.MarkError()
		return fmt.Errorf("claude web error: HTTP %d", resp.StatusCode)
	}

	// Stream response back as Anthropic SSE
	if req.Stream {
		err = StreamClaudeWebToAnthropic(ctx, w, resp.Body, req.Model)
	} else {
		err = r.collectAndRespond(w, resp.Body, req.Model)
	}

	if err != nil {
		sess.MarkError()
		return err
	}
	sess.Release()
	return nil
}

func (r *Router) collectAndRespond(w http.ResponseWriter, body io.Reader, model string) error {
	events := session.ParseSSEStream(body)
	var fullText strings.Builder
	for event := range events {
		if event.Data == "" {
			continue
		}
		var data map[string]any
		if err := json.Unmarshal([]byte(event.Data), &data); err != nil {
			continue
		}
		if text, ok := data["completion"].(string); ok {
			fullText.WriteString(text)
		}
	}

	resp := BuildAnthropicResponse(model, fullText.String(), EstimateTokens(fullText.String()), EstimateTokens(fullText.String()))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(resp)
}

func (r *Router) executePeer(ctx context.Context, w http.ResponseWriter, p *peer.PeerInfo, reqBody []byte) (*RouteResult, error) {
	client := peer.NewClient(p)
	result := &RouteResult{Via: "peer:" + p.Name}

	resp, err := client.ForwardRequest(ctx, reqBody)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		r.registry.MarkUnavailable(p.ID)
		return nil, fmt.Errorf("peer rate limited")
	}

	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)
	return result, err
}

func (r *Router) executeFallback(ctx context.Context, w http.ResponseWriter, reqBody []byte, req *session.AnthropicRequest) (*RouteResult, error) {
	fb := r.fallback.BestAvailable()
	if fb == nil {
		return nil, fmt.Errorf("no fallback available")
	}

	result := &RouteResult{Via: fb.Name(), Model: fb.DefaultModel()}
	err := fb.Forward(ctx, w, reqBody, req)
	return result, err
}
