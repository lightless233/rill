package handler

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRefreshAllRunGuard(t *testing.T) {
	h := &Handler{}

	if !h.tryStartRefreshAll() {
		t.Fatal("expected first refresh-all start to succeed")
	}
	if h.tryStartRefreshAll() {
		t.Fatal("expected second refresh-all start to be rejected")
	}

	h.finishRefreshAll()

	if !h.tryStartRefreshAll() {
		t.Fatal("expected refresh-all to be allowed after finish")
	}
}

type refreshCall struct {
	feedID      int64
	hasDeadline bool
}

type contextRecordingPuller struct {
	recordingPuller
	calls chan refreshCall
}

func (p *contextRecordingPuller) RefreshFeed(ctx context.Context, feedID int64) error {
	_, hasDeadline := ctx.Deadline()
	p.calls <- refreshCall{feedID: feedID, hasDeadline: hasDeadline}
	return nil
}

func TestManualAndInitialRefreshPassUnboundedParentContext(t *testing.T) {
	puller := &contextRecordingPuller{calls: make(chan refreshCall, 4)}
	h, st := newAnonTestHandler(t, &puller.recordingPuller)
	defer st.Close()
	h.config.AllowPrivateFeeds = true
	h.puller = puller

	r := h.SetupRouter()

	// Create feed (initial pull)
	w := performRequest(
		r,
		http.MethodPost,
		"/api/feeds",
		strings.NewReader(`{"group_id":1,"name":"Feed A","link":"http://127.0.0.1/a.xml"}`),
		map[string]string{"Content-Type": "application/json"},
	)
	if w.Code != http.StatusOK {
		t.Fatalf("create feed status = %d, body = %s", w.Code, w.Body.String())
	}

	// Manual refresh feed
	w = performRequest(r, http.MethodPost, "/api/feeds/1/refresh", nil, nil)
	if w.Code != http.StatusAccepted {
		t.Fatalf("refresh feed status = %d, body = %s", w.Code, w.Body.String())
	}

	// Batch create feeds (initial pull)
	w = performRequest(
		r,
		http.MethodPost,
		"/api/feeds/batch",
		strings.NewReader(`{"feeds":[{"group_id":1,"name":"Feed B","link":"http://127.0.0.1/b.xml"}]}`),
		map[string]string{"Content-Type": "application/json"},
	)
	if w.Code != http.StatusOK {
		t.Fatalf("batch create feeds status = %d, body = %s", w.Code, w.Body.String())
	}

	for i := range 3 {
		select {
		case call := <-puller.calls:
			if call.hasDeadline {
				t.Fatalf("call %d (feed %d): expected parent context without queue-draining deadline", i, call.feedID)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for RefreshFeed call %d", i)
		}
	}
}
