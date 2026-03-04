package fallback

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/GURSEWAK13/claude-connector/internal/config"
	"github.com/GURSEWAK13/claude-connector/internal/session"
)

// Backend is a fallback inference backend.
type Backend interface {
	Name() string
	DefaultModel() string
	IsAvailable() bool
	Forward(ctx context.Context, w http.ResponseWriter, body []byte, req *session.AnthropicRequest) error
}

// Manager manages fallback backends.
type Manager struct {
	cfg      *config.Config
	backends []Backend
}

func NewManager(cfg *config.Config) *Manager {
	m := &Manager{cfg: cfg}

	if cfg.Fallback.OllamaEnabled {
		m.backends = append(m.backends, NewOllamaBackend(cfg))
	}
	if cfg.Fallback.LMStudioEnabled {
		m.backends = append(m.backends, NewLMStudioBackend(cfg))
	}

	return m
}

// Start runs health checks on all backends.
func (m *Manager) Start(ctx context.Context) error {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Initial check
	m.checkAll(ctx)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			m.checkAll(ctx)
		}
	}
}

func (m *Manager) checkAll(ctx context.Context) {
	for _, b := range m.backends {
		go func(backend Backend) {
			checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			_ = checkHealth(checkCtx, backend)
		}(b)
	}
}

func checkHealth(ctx context.Context, b Backend) bool {
	_ = ctx
	return b.IsAvailable()
}

// BestAvailable returns the first available backend, or nil if none are available.
func (m *Manager) BestAvailable() Backend {
	for _, b := range m.backends {
		if b.IsAvailable() {
			return b
		}
	}
	return nil
}

// Statuses returns availability info for all backends.
func (m *Manager) Statuses() []BackendStatus {
	var out []BackendStatus
	for _, b := range m.backends {
		out = append(out, BackendStatus{
			Name:      b.Name(),
			Available: b.IsAvailable(),
		})
	}
	return out
}

type BackendStatus struct {
	Name      string
	Available bool
}

// probeURL checks if an HTTP endpoint is reachable.
func probeURL(url string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode < 500
}

// formatError formats an error as an Anthropic error JSON.
func formatError(msg string) string {
	return fmt.Sprintf(`{"type":"error","error":{"type":"api_error","message":%q}}`, msg)
}
