package web

import (
	"context"
	"encoding/json"
	"sync"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// EventType identifies WebSocket event types.
type EventType string

const (
	EventPeerUpdate    EventType = "peer_update"
	EventSessionUpdate EventType = "session_update"
	EventRequestLog    EventType = "request_log"
	EventMetrics       EventType = "metrics_update"
)

// Event is a JSON message sent over WebSocket.
type Event struct {
	Type    EventType `json:"type"`
	Payload any       `json:"payload"`
}

// Hub manages WebSocket connections and broadcasts events.
type Hub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]struct{}
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[*websocket.Conn]struct{}),
	}
}

// Register adds a WebSocket client.
func (h *Hub) Register(conn *websocket.Conn) {
	h.mu.Lock()
	h.clients[conn] = struct{}{}
	h.mu.Unlock()
}

// Unregister removes a WebSocket client.
func (h *Hub) Unregister(conn *websocket.Conn) {
	h.mu.Lock()
	delete(h.clients, conn)
	h.mu.Unlock()
}

// Broadcast sends an event to all connected clients.
func (h *Hub) Broadcast(ctx context.Context, evt Event) {
	h.mu.RLock()
	conns := make([]*websocket.Conn, 0, len(h.clients))
	for c := range h.clients {
		conns = append(conns, c)
	}
	h.mu.RUnlock()

	for _, c := range conns {
		go func(conn *websocket.Conn) {
			writeCtx, cancel := context.WithTimeout(ctx, 5e9) // 5 seconds
			defer cancel()
			if err := wsjson.Write(writeCtx, conn, evt); err != nil {
				h.Unregister(conn)
			}
		}(c)
	}
}

// BroadcastRaw sends raw JSON bytes to all clients.
func (h *Hub) BroadcastRaw(ctx context.Context, evtType EventType, payload any) {
	h.Broadcast(ctx, Event{Type: evtType, Payload: payload})
}

// MarshalEvent serializes an event to JSON (for logging).
func MarshalEvent(evt Event) ([]byte, error) {
	return json.Marshal(evt)
}
