package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/GURSEWAK13/claude-connector/internal/session"
)

// MessagesHandler handles POST /v1/messages
type MessagesHandler struct {
	router *Router
	events chan<- RequestEvent
}

// RequestEvent records routing outcomes for the TUI/Web dashboard.
type RequestEvent struct {
	Via      string
	Model    string
	Status   int
	Duration string
	Error    string
}

func NewMessagesHandler(router *Router, events chan<- RequestEvent) *MessagesHandler {
	return &MessagesHandler{router: router, events: events}
}

func (h *MessagesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20)) // 10 MB limit
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"read body: %v"}`, err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	req, err := session.ParseAnthropicRequest(body)
	if err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	result, routeErr := h.router.RouteRequest(r.Context(), w, body, req)

	event := RequestEvent{Model: req.Model}
	if result != nil {
		event.Via = result.Via
		event.Duration = result.Duration.Round(10 * 1e6).String()
		event.Status = 200
	}
	if routeErr != nil {
		event.Error = routeErr.Error()
		event.Status = 429
	}

	select {
	case h.events <- event:
	default:
	}
}

// ModelsHandler returns a static list of available models.
type ModelsHandler struct{}

func (h *ModelsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"object": "list",
		"data": []map[string]any{
			{"id": "claude-3-5-sonnet-20241022", "object": "model"},
			{"id": "claude-3-5-haiku-20241022", "object": "model"},
			{"id": "claude-3-opus-20240229", "object": "model"},
			{"id": "claude-3-haiku-20240307", "object": "model"},
		},
	})
}
