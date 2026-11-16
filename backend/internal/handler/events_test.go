package handler

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/0x2E/fusion/internal/eventhub"
)

func readSSEEvent(t *testing.T, reader *bufio.Reader) (string, string) {
	t.Helper()

	eventName := "message"
	dataLines := []string{}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read SSE line: %v", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if len(dataLines) > 0 {
				return eventName, strings.Join(dataLines, "\n")
			}
			eventName = "message"
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
}

func TestEventsStreamRequiresAuth(t *testing.T) {
	h, _ := newFeverTestHandler(t)
	r := h.SetupRouter()

	w := performRequest(r, http.MethodGet, "/api/events", nil, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/events without session status = %d, want 401", w.Code)
	}
}

func TestEventsStreamSendsReadyAndFeedChanges(t *testing.T) {
	h, _ := newAnonTestHandler(t, nil)
	hub := eventhub.New()
	defer hub.Close()
	h.SetEventHub(hub)

	server := httptest.NewServer(h.SetupRouter())
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Get(server.URL + "/api/events")
	if err != nil {
		t.Fatalf("GET /api/events: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/events status = %d", response.StatusCode)
	}
	if got := response.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := response.Header.Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("Cache-Control = %q", got)
	}

	reader := bufio.NewReader(response.Body)
	eventName, data := readSSEEvent(t, reader)
	if eventName != "ready" || data != "{}" {
		t.Fatalf("first event = %q %q, want ready {}", eventName, data)
	}

	hub.Notify()
	eventName, data = readSSEEvent(t, reader)
	if eventName != "feeds_changed" || data != "{}" {
		t.Fatalf("notification event = %q %q, want feeds_changed {}", eventName, data)
	}

	hub.Close()
	if _, err := reader.ReadString('\n'); err != io.EOF {
		t.Fatalf("stream did not close after hub close: %v", err)
	}
}
