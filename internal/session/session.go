package session

import (
	"sync"
	"time"
)

// State represents the session state machine states.
type State int

const (
	StateAvailable    State = iota
	StateInUse
	StateRateLimited
	StateCoolingDown
)

func (s State) String() string {
	switch s {
	case StateAvailable:
		return "AVAILABLE"
	case StateInUse:
		return "IN_USE"
	case StateRateLimited:
		return "RATE_LIMITED"
	case StateCoolingDown:
		return "COOLING_DOWN"
	default:
		return "UNKNOWN"
	}
}

// Backoff levels for cooling-down duration (exponential)
var backoffLevels = []time.Duration{
	60 * time.Second,
	120 * time.Second,
	240 * time.Second,
	600 * time.Second,
}

// Session represents a single Claude session (web or API key).
type Session struct {
	mu sync.RWMutex

	ID         string
	Type       string // "claude_web" | "anthropic_api"
	SessionKey string
	Enabled    bool

	state          State
	backoffLevel   int
	coolingUntil   time.Time
	rateLimitedAt  time.Time
	useCount       int
	errorCount     int
	lastUsed       time.Time
}

func NewSession(id, sessionType, key string, enabled bool) *Session {
	return &Session{
		ID:         id,
		Type:       sessionType,
		SessionKey: key,
		Enabled:    enabled,
		state:      StateAvailable,
	}
}

// State returns the current state (with cooling-down expiry check).
func (s *Session) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == StateCoolingDown && time.Now().After(s.coolingUntil) {
		s.state = StateAvailable
		s.backoffLevel = 0
	}
	return s.state
}

// IsAvailable returns true if the session can accept a new request.
func (s *Session) IsAvailable() bool {
	return s.Enabled && s.State() == StateAvailable
}

// Acquire transitions AVAILABLE → IN_USE. Returns false if unavailable.
func (s *Session) Acquire() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Refresh cooling-down state
	if s.state == StateCoolingDown && time.Now().After(s.coolingUntil) {
		s.state = StateAvailable
		s.backoffLevel = 0
	}

	if s.state != StateAvailable || !s.Enabled {
		return false
	}
	s.state = StateInUse
	s.lastUsed = time.Now()
	s.useCount++
	return true
}

// Release transitions IN_USE → AVAILABLE after a successful request.
func (s *Session) Release() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == StateInUse {
		s.state = StateAvailable
	}
}

// MarkRateLimited transitions to RATE_LIMITED → COOLING_DOWN.
func (s *Session) MarkRateLimited(retryAfter time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state = StateRateLimited
	s.rateLimitedAt = time.Now()
	s.errorCount++

	// Pick backoff level
	level := s.backoffLevel
	if level >= len(backoffLevels) {
		level = len(backoffLevels) - 1
	}

	cooldown := backoffLevels[level]
	if retryAfter > cooldown {
		cooldown = retryAfter
	}

	s.backoffLevel++
	s.state = StateCoolingDown
	s.coolingUntil = time.Now().Add(cooldown)
}

// MarkError records a non-rate-limit error and releases the session.
func (s *Session) MarkError() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.errorCount++
	if s.state == StateInUse {
		s.state = StateAvailable
	}
}

// CoolingUntil returns the time until the session exits cooling-down.
func (s *Session) CoolingUntil() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.coolingUntil
}

// CooldownRemaining returns how long until the session is available again.
func (s *Session) CooldownRemaining() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.state != StateCoolingDown {
		return 0
	}
	remaining := time.Until(s.coolingUntil)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// Stats returns usage statistics.
func (s *Session) Stats() (useCount, errorCount int, lastUsed time.Time) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.useCount, s.errorCount, s.lastUsed
}

// Snapshot returns a read-only snapshot of the session state for display.
type Snapshot struct {
	ID               string
	Type             string
	Enabled          bool
	State            State
	CooldownRemaining time.Duration
	UseCount         int
	ErrorCount       int
	LastUsed         time.Time
}

func (s *Session) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Refresh cooling-down
	if s.state == StateCoolingDown && time.Now().After(s.coolingUntil) {
		s.state = StateAvailable
		s.backoffLevel = 0
	}

	var remaining time.Duration
	if s.state == StateCoolingDown {
		remaining = time.Until(s.coolingUntil)
		if remaining < 0 {
			remaining = 0
		}
	}

	return Snapshot{
		ID:               s.ID,
		Type:             s.Type,
		Enabled:          s.Enabled,
		State:            s.state,
		CooldownRemaining: remaining,
		UseCount:         s.useCount,
		ErrorCount:       s.errorCount,
		LastUsed:         s.lastUsed,
	}
}
