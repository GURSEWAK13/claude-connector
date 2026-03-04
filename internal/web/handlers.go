package web

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/GURSEWAK13/claude-connector/internal/config"
	"github.com/GURSEWAK13/claude-connector/internal/fallback"
	"github.com/GURSEWAK13/claude-connector/internal/peer"
	"github.com/GURSEWAK13/claude-connector/internal/session"
)

// apiHandlers holds the dependencies for REST API handlers.
type apiHandlers struct {
	cfg       *config.Config
	pool      *session.Pool
	registry  *peer.Registry
	fb        *fallback.Manager
	startTime time.Time
}

func newAPIHandlers(cfg *config.Config, pool *session.Pool, registry *peer.Registry, fb *fallback.Manager) *apiHandlers {
	return &apiHandlers{
		cfg:       cfg,
		pool:      pool,
		registry:  registry,
		fb:        fb,
		startTime: time.Now(),
	}
}

func (h *apiHandlers) handleStatus(w http.ResponseWriter, r *http.Request) {
	type statusResponse struct {
		NodeID    string `json:"node_id"`
		NodeName  string `json:"node_name"`
		Uptime    int64  `json:"uptime_seconds"`
		ProxyPort int    `json:"proxy_port"`
		PeerPort  int    `json:"peer_port"`
		WebPort   int    `json:"web_port"`

		Sessions  []session.Snapshot       `json:"sessions"`
		Peers     []peer.PeerInfo          `json:"peers"`
		Fallbacks []fallback.BackendStatus `json:"fallbacks"`
	}

	resp := statusResponse{
		NodeID:    h.cfg.NodeID,
		NodeName:  h.cfg.NodeName,
		Uptime:    int64(time.Since(h.startTime).Seconds()),
		ProxyPort: h.cfg.Proxy.Port,
		PeerPort:  h.cfg.Peer.Port,
		WebPort:   h.cfg.Web.Port,
		Sessions:  h.pool.Snapshots(),
		Peers:     h.registry.All(),
		Fallbacks: h.fb.Statuses(),
	}

	writeJSON(w, resp)
}

func (h *apiHandlers) handlePeers(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.registry.All())
}

func (h *apiHandlers) handleSessions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.pool.Snapshots())
}

func (h *apiHandlers) handleAddSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var entry config.SessionEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.pool.AddSession(entry); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	writeJSON(w, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
