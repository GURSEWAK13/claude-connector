package proxy

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/GURSEWAK13/claude-connector/internal/config"
)

// Server is the Anthropic-compatible proxy HTTP server.
type Server struct {
	cfg    *config.Config
	router *Router
	events chan RequestEvent
}

func NewServer(cfg *config.Config, router *Router) *Server {
	return &Server{
		cfg:    cfg,
		router: router,
		events: make(chan RequestEvent, 256),
	}
}

// Events returns the channel for request events (for TUI / web consumers).
func (s *Server) Events() <-chan RequestEvent {
	return s.events
}

func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	handler := NewMessagesHandler(s.router, s.events)
	mux.Handle("/v1/messages", handler)
	mux.Handle("/v1/models", &ModelsHandler{})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	var h http.Handler = mux
	if s.cfg.Proxy.APIKey != "" {
		h = authMiddleware(s.cfg.Proxy.APIKey, mux)
	}

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", s.cfg.Proxy.Port),
		Handler:      h,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second,
		IdleTimeout:  120 * time.Second,
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
		return fmt.Errorf("proxy server: %w", err)
	}
}

func authMiddleware(apiKey string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-API-Key")
		if key == "" {
			if bearer := r.Header.Get("Authorization"); len(bearer) > 7 {
				key = bearer[7:] // strip "Bearer "
			}
		}
		if key != apiKey {
			http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
