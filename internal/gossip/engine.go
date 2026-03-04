package gossip

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/GURSEWAK13/claude-connector/internal/config"
	"github.com/GURSEWAK13/claude-connector/internal/peer"
	"github.com/GURSEWAK13/claude-connector/internal/session"
)

const (
	gossipInterval = 5 * time.Second
	gossipFanout   = 3 // send to N random peers per round
)

// Engine runs periodic gossip rounds, broadcasting status to random peers.
type Engine struct {
	cfg          *config.Config
	registry     *peer.Registry
	pool         *session.Pool
	startTime    time.Time
	requestCount int64
	httpClient   *http.Client
}

func NewEngine(cfg *config.Config, registry *peer.Registry, pool *session.Pool) *Engine {
	return &Engine{
		cfg:       cfg,
		registry:  registry,
		pool:      pool,
		startTime: time.Now(),
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// Start runs the gossip engine until the context is cancelled.
func (e *Engine) Start(ctx context.Context) error {
	ticker := time.NewTicker(gossipInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			e.gossipRound(ctx)
		}
	}
}

func (e *Engine) gossipRound(ctx context.Context) {
	peers := e.registry.All()
	if len(peers) == 0 {
		return
	}

	// Pick up to gossipFanout random peers
	targets := pickRandom(peers, gossipFanout)

	msg := e.buildStatusMessage()
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	for _, p := range targets {
		go func(p peer.PeerInfo) {
			if err := e.sendGossip(ctx, p, data); err != nil {
				// Non-fatal; just log
				_ = fmt.Sprintf("gossip to %s: %v", p.Name, err)
			}
		}(p)
	}
}

func (e *Engine) sendGossip(ctx context.Context, p peer.PeerInfo, data []byte) error {
	url := fmt.Sprintf("http://%s:%d/peer/v1/gossip", p.Host, p.Port)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gossip HTTP %d", resp.StatusCode)
	}

	// Parse peer's reply and update registry
	var reply Message
	if err := json.NewDecoder(resp.Body).Decode(&reply); err != nil {
		return nil // non-fatal
	}

	if reply.Status != nil {
		e.registry.Upsert(peer.PeerInfo{
			ID:                reply.Status.NodeID,
			Name:              reply.Status.NodeName,
			Host:              p.Host,
			Port:              p.Port,
			AvailableSessions: reply.Status.AvailableSessions,
			Available:         reply.Status.AvailableSessions > 0,
		})
	}

	return nil
}

func (e *Engine) buildStatusMessage() Message {
	return Message{
		Type:      MsgStatusUpdate,
		SenderID:  e.cfg.NodeID,
		Timestamp: time.Now(),
		Status: &NodeStatus{
			NodeID:            e.cfg.NodeID,
			NodeName:          e.cfg.NodeName,
			AvailableSessions: e.pool.AvailableCount(),
			RequestCount:      atomic.LoadInt64(&e.requestCount),
			Uptime:            int64(time.Since(e.startTime).Seconds()),
			UpdatedAt:         time.Now(),
		},
	}
}

// IncrementRequests increments the request counter.
func (e *Engine) IncrementRequests() {
	atomic.AddInt64(&e.requestCount, 1)
}

func pickRandom(peers []peer.PeerInfo, n int) []peer.PeerInfo {
	if len(peers) <= n {
		return peers
	}
	// Fisher-Yates partial shuffle
	indices := rand.Perm(len(peers))[:n]
	result := make([]peer.PeerInfo, n)
	for i, idx := range indices {
		result[i] = peers[idx]
	}
	return result
}
