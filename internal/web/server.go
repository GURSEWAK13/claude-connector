package web

import (
	"context"
	"embed"
	"fmt"
	"net/http"
	"time"

	"nhooyr.io/websocket"
	"github.com/GURSEWAK13/claude-connector/internal/config"
	"github.com/GURSEWAK13/claude-connector/internal/fallback"
	"github.com/GURSEWAK13/claude-connector/internal/peer"
	"github.com/GURSEWAK13/claude-connector/internal/session"
)

//go:embed index.html app.js style.css d3.v7.min.js
var staticFiles embed.FS

// Server serves the web dashboard.
type Server struct {
	cfg      *config.Config
	pool     *session.Pool
	registry *peer.Registry
	fb       *fallback.Manager
	hub      *Hub
	handlers *apiHandlers
}

func NewServer(cfg *config.Config, pool *session.Pool, registry *peer.Registry) *Server {
	return &Server{
		cfg:      cfg,
		pool:     pool,
		registry: registry,
		hub:      NewHub(),
	}
}

// SetFallback sets the fallback manager (called after construction to avoid circular deps).
func (s *Server) SetFallback(fb *fallback.Manager) {
	s.fb = fb
	s.handlers = newAPIHandlers(s.cfg, s.pool, s.registry, s.fb)
}

func (s *Server) Start(ctx context.Context) error {
	if s.fb == nil {
		// Initialize with a nil-safe fallback manager placeholder
		s.fb = fallback.NewManager(s.cfg)
	}
	if s.handlers == nil {
		s.handlers = newAPIHandlers(s.cfg, s.pool, s.registry, s.fb)
	}

	mux := http.NewServeMux()

	// REST API
	mux.HandleFunc("/api/status", s.handlers.handleStatus)
	mux.HandleFunc("/api/peers", s.handlers.handlePeers)
	mux.HandleFunc("/api/sessions", s.handlers.handleSessions)
	mux.HandleFunc("/api/sessions/add", s.handlers.handleAddSession)

	// WebSocket
	mux.HandleFunc("/ws", s.handleWebSocket)

	// Serve static frontend (embedded)
	mux.Handle("/", http.FileServer(http.FS(staticFiles)))

	// Start broadcasting loop
	go s.broadcastLoop(ctx)

	addr := fmt.Sprintf("%s:%d", s.cfg.Web.BindAddress, s.cfg.Web.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
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
		return fmt.Errorf("web server: %w", err)
	}
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // Allow LAN connections
	})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	s.hub.Register(conn)
	defer s.hub.Unregister(conn)

	// Send initial state
	s.hub.BroadcastRaw(r.Context(), EventSessionUpdate, s.pool.Snapshots())
	s.hub.BroadcastRaw(r.Context(), EventPeerUpdate, s.registry.All())

	// Keep alive: read until client disconnects
	for {
		_, _, err := conn.Read(r.Context())
		if err != nil {
			break
		}
	}
}

func (s *Server) broadcastLoop(ctx context.Context) {
	sessionCh := s.pool.Subscribe()
	peerCh := s.registry.Subscribe()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case snap := <-sessionCh:
			s.hub.BroadcastRaw(ctx, EventSessionUpdate, snap)
		case info := <-peerCh:
			s.hub.BroadcastRaw(ctx, EventPeerUpdate, info)
		case <-ticker.C:
			// Periodic metrics update
			s.hub.BroadcastRaw(ctx, EventMetrics, map[string]any{
				"sessions":  s.pool.Snapshots(),
				"peers":     s.registry.All(),
				"fallbacks": s.fb.Statuses(),
			})
		}
	}
}
