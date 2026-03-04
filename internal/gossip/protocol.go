package gossip

import "time"

// MessageType identifies the type of gossip message.
type MessageType string

const (
	MsgStatusUpdate MessageType = "status_update"
	MsgPeerList     MessageType = "peer_list"
)

// Message is a gossip protocol message.
type Message struct {
	Type      MessageType `json:"type"`
	SenderID  string      `json:"sender_id"`
	Timestamp time.Time   `json:"timestamp"`

	// For MsgStatusUpdate
	Status *NodeStatus `json:"status,omitempty"`

	// For MsgPeerList
	Peers []PeerSummary `json:"peers,omitempty"`
}

// NodeStatus is a node's current operational status.
type NodeStatus struct {
	NodeID            string    `json:"node_id"`
	NodeName          string    `json:"node_name"`
	AvailableSessions int       `json:"available_sessions"`
	TotalSessions     int       `json:"total_sessions"`
	RequestCount      int64     `json:"request_count"`
	Uptime            int64     `json:"uptime_seconds"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// PeerSummary is a condensed peer entry suitable for gossip exchange.
type PeerSummary struct {
	ID                string        `json:"id"`
	Name              string        `json:"name"`
	Host              string        `json:"host"`
	Port              int           `json:"port"`
	AvailableSessions int           `json:"available_sessions"`
	Latency           time.Duration `json:"latency_ns"`
}
