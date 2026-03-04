package peer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client forwards requests to a peer's API server.
type Client struct {
	peer       PeerInfo
	httpClient *http.Client
	auth       *Authenticator
}

func NewClient(peer *PeerInfo) *Client {
	return &Client{
		peer: *peer,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// ForwardRequest sends a request body to the peer and returns the raw response.
func (c *Client) ForwardRequest(ctx context.Context, body []byte) (*http.Response, error) {
	url := fmt.Sprintf("http://%s:%d/peer/v1/forward", c.peer.Host, c.peer.Port)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Node-ID", "self")

	if c.auth != nil {
		bodyHash := HashBody(body)
		if err := c.auth.Sign(req, bodyHash); err != nil {
			return nil, fmt.Errorf("sign request: %w", err)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("forward request to %s: %w", c.peer.Name, err)
	}

	return resp, nil
}

// GetStatus fetches the peer's current status.
func (c *Client) GetStatus(ctx context.Context) (*PeerStatus, error) {
	url := fmt.Sprintf("http://%s:%d/peer/v1/status", c.peer.Host, c.peer.Port)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get status from %s: %w", c.peer.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d from peer", resp.StatusCode)
	}

	var status PeerStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, err
	}
	return &status, nil
}

// PeerStatus is the response from GET /peer/v1/status.
type PeerStatus struct {
	NodeID            string `json:"node_id"`
	NodeName          string `json:"node_name"`
	AvailableSessions int    `json:"available_sessions"`
	RequestCount      int    `json:"request_count"`
}
