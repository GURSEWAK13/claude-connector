package session

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/GURSEWAK13/claude-connector/internal/config"
)

// Pool manages a set of sessions.
type Pool struct {
	mu       sync.RWMutex
	sessions []*Session
	cfg      *config.Config

	// Subscribers for state change events
	subs   []chan Snapshot
	subsMu sync.Mutex
}

func NewPool(cfg *config.Config) *Pool {
	p := &Pool{cfg: cfg}
	for _, entry := range cfg.Sessions.Sessions {
		s := NewSession(entry.ID, entry.Type, entry.SessionKey, entry.Enabled)
		p.sessions = append(p.sessions, s)
	}
	return p
}

// Start runs background tasks (cooling-down monitor, etc.)
func (p *Pool) Start(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			p.broadcastSnapshots()
		}
	}
}

func (p *Pool) broadcastSnapshots() {
	p.mu.RLock()
	snaps := make([]Snapshot, len(p.sessions))
	for i, s := range p.sessions {
		snaps[i] = s.Snapshot()
	}
	p.mu.RUnlock()

	p.subsMu.Lock()
	defer p.subsMu.Unlock()

	for _, snap := range snaps {
		for _, ch := range p.subs {
			select {
			case ch <- snap:
			default:
			}
		}
	}
}

// Subscribe returns a channel that receives session snapshots on state changes.
func (p *Pool) Subscribe() chan Snapshot {
	ch := make(chan Snapshot, 32)
	p.subsMu.Lock()
	p.subs = append(p.subs, ch)
	p.subsMu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel.
func (p *Pool) Unsubscribe(ch chan Snapshot) {
	p.subsMu.Lock()
	defer p.subsMu.Unlock()

	for i, c := range p.subs {
		if c == ch {
			p.subs = append(p.subs[:i], p.subs[i+1:]...)
			return
		}
	}
}

// Acquire returns the first available session, or nil if none are available.
func (p *Pool) Acquire() *Session {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, s := range p.sessions {
		if s.Acquire() {
			return s
		}
	}
	return nil
}

// AcquireExcluding returns the first available session not in the excluded set.
func (p *Pool) AcquireExcluding(excluded map[string]bool) *Session {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, s := range p.sessions {
		if excluded[s.ID] {
			continue
		}
		if s.Acquire() {
			return s
		}
	}
	return nil
}

// Snapshots returns snapshots of all sessions.
func (p *Pool) Snapshots() []Snapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()

	snaps := make([]Snapshot, len(p.sessions))
	for i, s := range p.sessions {
		snaps[i] = s.Snapshot()
	}
	return snaps
}

// AvailableCount returns the number of available sessions.
func (p *Pool) AvailableCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	count := 0
	for _, s := range p.sessions {
		if s.IsAvailable() {
			count++
		}
	}
	return count
}

// AddSession adds a new session to the pool at runtime.
func (p *Pool) AddSession(entry config.SessionEntry) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, s := range p.sessions {
		if s.ID == entry.ID {
			return fmt.Errorf("session %q already exists", entry.ID)
		}
	}

	s := NewSession(entry.ID, entry.Type, entry.SessionKey, entry.Enabled)
	p.sessions = append(p.sessions, s)
	return nil
}

// GetSession returns a session by ID.
func (p *Pool) GetSession(id string) (*Session, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, s := range p.sessions {
		if s.ID == id {
			return s, true
		}
	}
	return nil, false
}
