package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// StreamAnthropicPassthrough copies an Anthropic-format SSE stream from upstream to the client.
// Used when forwarding from a peer that already speaks Anthropic API.
func StreamAnthropicPassthrough(ctx context.Context, w http.ResponseWriter, body io.Reader) error {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, hasFlusher := w.(http.Flusher)

	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
		if hasFlusher && line == "" {
			flusher.Flush()
		}
	}

	return scanner.Err()
}

// StreamClaudeWebToAnthropic translates a Claude.ai SSE stream into Anthropic SSE format
// and writes it to the http.ResponseWriter.
func StreamClaudeWebToAnthropic(ctx context.Context, w http.ResponseWriter, body io.Reader, model string) error {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, hasFlusher := w.(http.Flusher)

	// Write message_start block
	startLines := ClaudeWebEventToAnthropicSSE("message_start", "")
	for _, l := range startLines {
		fmt.Fprintln(w, l)
	}
	if hasFlusher {
		flusher.Flush()
	}

	scanner := bufio.NewScanner(body)
	var eventType string
	var dataLines []string

	flushEvent := func() {
		if len(dataLines) == 0 {
			return
		}
		data := strings.Join(dataLines, "\n")
		lines := ClaudeWebEventToAnthropicSSE(eventType, data)
		for _, l := range lines {
			fmt.Fprintln(w, l)
		}
		if hasFlusher && len(lines) > 0 {
			flusher.Flush()
		}
		eventType = ""
		dataLines = nil
	}

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()
		if line == "" {
			flushEvent()
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}

	flushEvent()

	// Write message_stop
	stopLines := ClaudeWebEventToAnthropicSSE("message_stop", "")
	for _, l := range stopLines {
		fmt.Fprintln(w, l)
	}
	if hasFlusher {
		flusher.Flush()
	}

	return scanner.Err()
}

// WriteSSEError writes an Anthropic-formatted error event to the response.
func WriteSSEError(w http.ResponseWriter, errType, message string) {
	w.Header().Set("Content-Type", "text/event-stream")
	fmt.Fprintf(w, "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":%q,\"message\":%q}}\n\n", errType, message)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}
