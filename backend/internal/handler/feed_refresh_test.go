package handler

import (
	"context"
	"encoding/json"
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

type blockingRefreshAllPuller struct {
	recordingPuller
	started chan struct{}
	release chan struct{}
}

func (p *blockingRefreshAllPuller) RefreshAll(ctx context.Context) (int, error) {
	close(p.started)
	select {
	case <-p.release:
	case <-ctx.Done():
		return 0, ctx.Err()
	}
	return 0, nil
}

func TestGetAppInfoReflectsRefreshAllRunning(t *testing.T) {
	puller := &blockingRefreshAllPuller{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	h, st := newAnonTestHandler(t, &puller.recordingPuller)
	defer st.Close()
	h.puller = puller

	r := h.SetupRouter()

	readRefreshAllRunning := func() bool {
		t.Helper()
		w := performRequest(r, http.MethodGet, "/api/app", nil, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("GET /api/app status = %d, body = %s", w.Code, w.Body.String())
		}
		var resp struct {
			Data appInfoResponse `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode /api/app response: %v", err)
		}
		return resp.Data.RefreshAllRunning
	}

	if readRefreshAllRunning() {
		t.Fatal("expected refresh_all_running=false initially")
	}

	w := performRequest(r, http.MethodPost, "/api/feeds/refresh", nil, nil)
	if w.Code != http.StatusAccepted {
		t.Fatalf("POST /api/feeds/refresh status = %d, body = %s", w.Code, w.Body.String())
	}

	select {
	case <-puller.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for RefreshAll to start")
	}

	if !readRefreshAllRunning() {
		t.Fatal("expected refresh_all_running=true while RefreshAll is running")
	}

	close(puller.release)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !readRefreshAllRunning() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected refresh_all_running to return to false after RefreshAll finished")
}
