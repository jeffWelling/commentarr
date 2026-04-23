package sse

import (
	"fmt"
	"net/http"
)

// Handler is the HTTP handler for /api/v1/events. It holds the
// connection open and writes each Event as an SSE frame until the
// client disconnects.
type Handler struct {
	broker *Broker
}

// NewHandler returns a Handler.
func NewHandler(b *Broker) *Handler { return &Handler{broker: b} }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	sub := h.broker.Subscribe()
	defer h.broker.Unsubscribe(sub)

	_, _ = fmt.Fprint(w, "event: hello\ndata: {}\n\n")
	flusher.Flush()

	for {
		select {
		case ev, ok := <-sub:
			if !ok {
				return
			}
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Kind, ev.Payload)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
