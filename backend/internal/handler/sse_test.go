package handler

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStartSSEStreamWritesEventsAndComments(t *testing.T) {
	recorder := httptest.NewRecorder()
	sendEvent, sendComment := startSSEStream(recorder)

	if err := sendEvent("ready", struct{}{}); err != nil {
		t.Fatalf("send event: %v", err)
	}
	if err := sendComment("heartbeat\nignored"); err != nil {
		t.Fatalf("send comment: %v", err)
	}

	if got := recorder.Header().Get("Content-Type"); got != "text/event-stream; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := recorder.Header().Get("X-Accel-Buffering"); got != "no" {
		t.Fatalf("X-Accel-Buffering = %q", got)
	}

	body := recorder.Body.String()
	if !strings.Contains(body, "event: ready\ndata: {}\n\n") {
		t.Fatalf("missing ready event: %q", body)
	}
	if !strings.Contains(body, ": heartbeat ignored\n\n") {
		t.Fatalf("missing sanitized comment heartbeat: %q", body)
	}
}
