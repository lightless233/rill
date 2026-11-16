package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// startSSEStream writes the common SSE response headers and returns an event
// sender plus a comment/heartbeat sender. The response deadline is extended on
// every write because the server-wide WriteTimeout is shorter than long-lived
// streams.
func startSSEStream(w http.ResponseWriter) (func(eventName string, payload any) error, func(text string) error) {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	rc := http.NewResponseController(w)
	extendDeadline := func() {
		_ = rc.SetWriteDeadline(time.Now().Add(3 * time.Minute))
	}

	sendEvent := func(eventName string, payload any) error {
		extendDeadline()
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventName, data); err != nil {
			return err
		}
		return rc.Flush()
	}

	sendComment := func(text string) error {
		extendDeadline()
		text = strings.ReplaceAll(text, "\n", " ")
		if _, err := fmt.Fprintf(w, ": %s\n\n", text); err != nil {
			return err
		}
		return rc.Flush()
	}

	return sendEvent, sendComment
}
