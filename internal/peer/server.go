package peer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/GURSEWAK13/claude-connector/internal/config"
	"github.com/GURSEWAK13/claude-connector/internal/session"
)

// Server is the peer API server (port 8767).
type Server struct {
	cfg          *config.Config
	pool         *session.Pool
	auth         *Authenticator
	requestCount int64
}

func NewServer(cfg *config.Config, pool *session.Pool) *Server {
	return &Server{
		cfg:  cfg,
		pool: pool,
		auth: NewAuthenticator(cfg.Peer.SharedSecret),
	}
}

func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/peer/v1/forward", s.handleForward)
	mux.HandleFunc("/peer/v1/status", s.handleStatus)
	mux.HandleFunc("/peer/v1/gossip", s.handleGossip)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", s.cfg.Peer.Port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutCtx)
	case err := <-errCh:
		return fmt.Errorf("peer server: %w", err)
	}
}

func (s *Server) handleForward(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Verify HMAC signature
	if err := s.auth.Verify(r, HashBody(body)); err != nil {
		http.Error(w, "unauthorized: "+err.Error(), http.StatusUnauthorized)
		return
	}

	// Acquire a local session
	sess := s.pool.Acquire()
	if sess == nil {
		http.Error(w, `{"type":"error","error":{"type":"rate_limit_error","message":"no sessions available"}}`,
			http.StatusTooManyRequests)
		return
	}

	atomic.AddInt64(&s.requestCount, 1)

	// Execute the request based on session type
	req, err := session.ParseAnthropicRequest(body)
	if err != nil {
		sess.MarkError()
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	switch sess.Type {
	case "anthropic_api":
		client := session.NewAnthropicClient(sess)
		resp, err := client.SendMessages(r.Context(), body)
		if err != nil {
			sess.MarkError()
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		if session.IsRateLimitResponse(resp.StatusCode) {
			retryAfter := session.ParseRetryAfter(resp)
			sess.MarkRateLimited(retryAfter)
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}

		for k, vs := range resp.Header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
		sess.Release()

	case "claude_web":
		// For simplicity in the peer case, we just return not-implemented
		// A full impl would use the claude_client.go
		sess.Release()
		http.Error(w, "claude_web sessions cannot be used as peer sessions yet", http.StatusNotImplemented)
	}

	_ = req
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	status := PeerStatus{
		NodeID:            s.cfg.NodeID,
		NodeName:          s.cfg.NodeName,
		AvailableSessions: s.pool.AvailableCount(),
		RequestCount:      int(atomic.LoadInt64(&s.requestCount)),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (s *Server) handleGossip(w http.ResponseWriter, r *http.Request) {
	// Gossip handler: accept peer status updates
	var msg GossipMessage
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	// Echo our own status back
	reply := GossipMessage{
		NodeID:            s.cfg.NodeID,
		NodeName:          s.cfg.NodeName,
		AvailableSessions: s.pool.AvailableCount(),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reply)
}

// GossipMessage is exchanged during gossip rounds.
type GossipMessage struct {
	NodeID            string `json:"node_id"`
	NodeName          string `json:"node_name"`
	AvailableSessions int    `json:"available_sessions"`
}
