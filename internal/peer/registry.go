package peer

import (
	"sync"
	"time"
)

// PeerInfo holds information about a discovered peer.
type PeerInfo struct {
	ID                string
	Name              string
	Host              string
	Port              int
	AvailableSessions int
	LastSeen          time.Time
	Latency           time.Duration
	Available         bool
}

// Registry tracks discovered peers.
type Registry struct {
	mu    sync.RWMutex
	peers map[string]*PeerInfo

	subs   []chan PeerInfo
	subsMu sync.Mutex
}

func NewRegistry() *Registry {
	return &Registry{
		peers: make(map[string]*PeerInfo),
	}
}

// Upsert adds or updates a peer in the registry.
func (r *Registry) Upsert(info PeerInfo) {
	r.mu.Lock()
	info.LastSeen = time.Now()
	r.peers[info.ID] = &info
	r.mu.Unlock()

	r.broadcast(info)
}

// MarkUnavailable flags a peer as unavailable (e.g., returned 429).
func (r *Registry) MarkUnavailable(id string) {
	r.mu.Lock()
	if p, ok := r.peers[id]; ok {
		p.Available = false
		p.AvailableSessions = 0
	}
	r.mu.Unlock()
}

// Remove removes a peer from the registry.
func (r *Registry) Remove(id string) {
	r.mu.Lock()
	delete(r.peers, id)
	r.mu.Unlock()
}

// All returns a snapshot of all known peers.
func (r *Registry) All() []PeerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]PeerInfo, 0, len(r.peers))
	for _, p := range r.peers {
		out = append(out, *p)
	}
	return out
}

// AvailablePeers returns peers that have available sessions, sorted by latency.
func (r *Registry) AvailablePeers() []PeerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []PeerInfo
	cutoff := time.Now().Add(-30 * time.Second)
	for _, p := range r.peers {
		if p.Available && p.AvailableSessions > 0 && p.LastSeen.After(cutoff) {
			out = append(out, *p)
		}
	}

	// Sort by latency (simple insertion sort for small N)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Latency < out[j-1].Latency; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}

	return out
}

// Subscribe returns a channel that receives peer updates.
func (r *Registry) Subscribe() chan PeerInfo {
	ch := make(chan PeerInfo, 32)
	r.subsMu.Lock()
	r.subs = append(r.subs, ch)
	r.subsMu.Unlock()
	return ch
}

func (r *Registry) broadcast(info PeerInfo) {
	r.subsMu.Lock()
	defer r.subsMu.Unlock()
	for _, ch := range r.subs {
		select {
		case ch <- info:
		default:
		}
	}
}
