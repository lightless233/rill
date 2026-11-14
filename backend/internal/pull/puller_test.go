package pull

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0x2E/fusion/internal/config"
	"github.com/0x2E/fusion/internal/store"
)

func TestRefreshFeedPreservesValidatorsWhen304OmitHeaders(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer st.Close()

	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)
		if count == 1 {
			w.Header().Set("Content-Type", "application/rss+xml")
			w.Header().Set("ETag", `"etag-v1"`)
			w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
			w.Header().Set("Cache-Control", "max-age=86400")
			w.Header().Set("Expires", "Tue, 03 Jan 2006 15:04:05 GMT")
			_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Demo</title><link>https://example.com</link>
<item><guid>g1</guid><title>Item</title><link>https://example.com/1</link></item>
</channel></rss>`)
			return
		}

		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	feed, err := st.CreateFeed(1, "Feed A", server.URL, "", "", nil)
	if err != nil {
		t.Fatalf("create feed: %v", err)
	}

	p := New(st, &config.Config{
		PullInterval:      1800,
		PullTimeout:       5,
		PullConcurrency:   1,
		PullMaxBackoff:    604800,
		AllowPrivateFeeds: true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.RefreshFeed(ctx, feed.ID); err != nil {
		t.Fatalf("first refresh: %v", err)
	}

	if err := p.RefreshFeed(ctx, feed.ID); err != nil {
		t.Fatalf("second refresh: %v", err)
	}

	updatedFeed, err := st.GetFeed(feed.ID)
	if err != nil {
		t.Fatalf("get feed: %v", err)
	}

	if updatedFeed.FetchState.ETag != `"etag-v1"` {
		t.Fatalf("etag = %q, want %q", updatedFeed.FetchState.ETag, `"etag-v1"`)
	}
	if updatedFeed.FetchState.LastModified != "Mon, 02 Jan 2006 15:04:05 GMT" {
		t.Fatalf("last_modified = %q, want %q", updatedFeed.FetchState.LastModified, "Mon, 02 Jan 2006 15:04:05 GMT")
	}
	if updatedFeed.FetchState.CacheControl != "max-age=86400" {
		t.Fatalf("cache_control = %q, want %q", updatedFeed.FetchState.CacheControl, "max-age=86400")
	}

	expires, err := http.ParseTime("Tue, 03 Jan 2006 15:04:05 GMT")
	if err != nil {
		t.Fatalf("parse expires: %v", err)
	}
	if updatedFeed.FetchState.ExpiresAt != expires.Unix() {
		t.Fatalf("expires_at = %d, want %d", updatedFeed.FetchState.ExpiresAt, expires.Unix())
	}
}

func TestRefreshAllWaitsForRunningJobs(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer st.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Demo</title><link>https://example.com</link>
<item><guid>%s</guid><title>Item</title><link>https://example.com%s</link></item>
</channel></rss>`, r.URL.Path, r.URL.Path)
	}))
	defer server.Close()

	if _, err := st.CreateFeed(1, "Feed A", server.URL+"/a", "", "", nil); err != nil {
		t.Fatalf("create feed A: %v", err)
	}
	if _, err := st.CreateFeed(1, "Feed B", server.URL+"/b", "", "", nil); err != nil {
		t.Fatalf("create feed B: %v", err)
	}

	p := New(st, &config.Config{
		PullInterval:      1800,
		PullTimeout:       5,
		PullConcurrency:   1,
		PullMaxBackoff:    604800,
		AllowPrivateFeeds: true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	count, err := p.RefreshAll(ctx)
	if err != nil {
		t.Fatalf("refresh all: %v", err)
	}
	if count != 2 {
		t.Fatalf("refresh count = %d, want 2", count)
	}

	feeds, err := st.ListFeeds()
	if err != nil {
		t.Fatalf("list feeds: %v", err)
	}
	for _, feed := range feeds {
		if feed.FetchState.LastSuccessAt <= 0 {
			t.Fatalf("feed %d last_success_at = %d, want > 0", feed.ID, feed.FetchState.LastSuccessAt)
		}
	}
}

func TestRefreshFeedBackfillsNameFromTitle(t *testing.T) {
	const rssWithTitle = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Tech Daily</title><link>https://example.com</link>
<item><guid>g1</guid><title>Item</title><link>https://example.com/1</link></item>
</channel></rss>`
	const rssWithoutTitle = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><link>https://example.com</link>
<item><guid>g1</guid><title>Item</title><link>https://example.com/1</link></item>
</channel></rss>`

	tests := []struct {
		name       string
		feedName   string
		serverBody string
		wantName   string
	}{
		{
			// Feeds added by URL default their name to the link; a successful
			// pull must replace it with the feed's own channel title.
			name:       "name equal to link is replaced by title",
			feedName:   "", // replaced with server.URL below
			serverBody: rssWithTitle,
			wantName:   "Tech Daily",
		},
		{
			name:       "custom name is kept",
			feedName:   "My Custom Name",
			serverBody: rssWithTitle,
			wantName:   "My Custom Name",
		},
		{
			name:       "feed without title keeps default name",
			feedName:   "", // replaced with server.URL below
			serverBody: rssWithoutTitle,
			wantName:   "", // replaced with server.URL below
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/rss+xml")
				_, _ = fmt.Fprint(w, tt.serverBody)
			}))
			defer server.Close()

			feedName := tt.feedName
			if feedName == "" {
				feedName = server.URL
			}
			wantName := tt.wantName
			if wantName == "" {
				wantName = server.URL
			}

			dbPath := filepath.Join(t.TempDir(), "test.db")
			st, err := store.New(dbPath)
			if err != nil {
				t.Fatalf("create store: %v", err)
			}
			defer st.Close()

			feed, err := st.CreateFeed(1, feedName, server.URL, "", "", nil)
			if err != nil {
				t.Fatalf("create feed: %v", err)
			}

			p := New(st, &config.Config{
				PullInterval:      1800,
				PullTimeout:       5,
				PullConcurrency:   1,
				PullMaxBackoff:    604800,
				AllowPrivateFeeds: true,
			})

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if err := p.RefreshFeed(ctx, feed.ID); err != nil {
				t.Fatalf("refresh feed: %v", err)
			}

			updated, err := st.GetFeed(feed.ID)
			if err != nil {
				t.Fatalf("get feed: %v", err)
			}
			if updated.Name != wantName {
				t.Fatalf("feed name = %q, want %q", updated.Name, wantName)
			}
		})
	}
}

func TestPullerAttemptTimeoutStartsAfterSemaphoreAcquisition(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer st.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/slow-success-a", "/slow-success-b":
			time.Sleep(160 * time.Millisecond)
		case "/attempt-timeout":
			select {
			case <-time.After(400 * time.Millisecond):
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Demo</title><link>https://example.com</link>
<item><guid>%s</guid><title>Item</title><link>https://example.com%s</link></item>
</channel></rss>`, r.URL.Path, r.URL.Path)
	}))
	defer server.Close()

	feedA, err := st.CreateFeed(1, "Feed A", server.URL+"/slow-success-a", "", "", nil)
	if err != nil {
		t.Fatalf("create feed A: %v", err)
	}
	feedB, err := st.CreateFeed(1, "Feed B", server.URL+"/slow-success-b", "", "", nil)
	if err != nil {
		t.Fatalf("create feed B: %v", err)
	}
	feedTimeout, err := st.CreateFeed(1, "Feed Timeout", server.URL+"/attempt-timeout", "", "", nil)
	if err != nil {
		t.Fatalf("create timeout feed: %v", err)
	}

	p := New(st, &config.Config{
		PullInterval:      1800,
		PullTimeout:       1,
		PullConcurrency:   1,
		PullMaxBackoff:    604800,
		AllowPrivateFeeds: true,
	})
	p.timeout = 240 * time.Millisecond

	// Two queued feeds under PullConcurrency=1 take ~320ms combined (>240ms),
	// so the second feed only succeeds if its attempt timeout starts after
	// semaphore acquisition rather than when queued.
	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	for _, id := range []int64{feedA.ID, feedB.ID} {
		wg.Add(1)
		go func(feedID int64) {
			defer wg.Done()
			errCh <- p.RefreshFeed(context.Background(), feedID)
		}(id)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("queued refresh failed unexpectedly: %v", err)
		}
	}

	for _, id := range []int64{feedA.ID, feedB.ID} {
		got, err := st.GetFeed(id)
		if err != nil {
			t.Fatalf("get feed %d: %v", id, err)
		}
		if got.FetchState.LastSuccessAt <= 0 || got.FetchState.ConsecutiveFailures != 0 {
			t.Fatalf("feed %d state = %+v, want success with 0 failures", id, got.FetchState)
		}
	}

	// Attempt exceeding p.timeout while parent context remains active must count as failure.
	if err := p.RefreshFeed(context.Background(), feedTimeout.ID); err != nil {
		t.Fatalf("timeout RefreshFeed returned unexpected error: %v", err)
	}
	gotTimeout, err := st.GetFeed(feedTimeout.ID)
	if err != nil {
		t.Fatalf("get timeout feed: %v", err)
	}
	if gotTimeout.FetchState.ConsecutiveFailures != 1 || gotTimeout.FetchState.LastError == "" {
		t.Fatalf("timeout feed state = %+v, want consecutive_failures=1 and non-empty last_error", gotTimeout.FetchState)
	}
}

func TestPullerParentCancellationDoesNotRecordFailure(t *testing.T) {
	t.Run("shutdown context cancel after HTTP dispatch avoids failure backoff", func(t *testing.T) {
		dbPath := filepath.Join(t.TempDir(), "test.db")
		st, err := store.New(dbPath)
		if err != nil {
			t.Fatalf("create store: %v", err)
		}
		defer st.Close()

		dispatched := make(chan struct{})
		var queuedHits atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/queued" {
				queuedHits.Add(1)
				return
			}
			close(dispatched)
			<-r.Context().Done()
		}))
		defer server.Close()

		inFlight, err := st.CreateFeed(1, "InFlight", server.URL+"/inflight", "", "", nil)
		if err != nil {
			t.Fatalf("create inflight feed: %v", err)
		}
		queued, err := st.CreateFeed(1, "Queued", server.URL+"/queued", "", "", nil)
		if err != nil {
			t.Fatalf("create queued feed: %v", err)
		}

		shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
		p := NewWithContext(shutdownCtx, st, &config.Config{
			PullInterval:      1800,
			PullTimeout:       5,
			PullConcurrency:   1,
			PullMaxBackoff:    604800,
			AllowPrivateFeeds: true,
		})

		done := make(chan error, 1)
		go func() {
			_, err := p.RefreshAll(context.Background())
			done <- err
		}()

		<-dispatched
		shutdownCancel()

		if err := <-done; err == nil {
			t.Fatal("expected RefreshAll to return cancellation error on shutdown")
		}

		for _, id := range []int64{inFlight.ID, queued.ID} {
			got, err := st.GetFeed(id)
			if err != nil {
				t.Fatalf("get feed %d: %v", id, err)
			}
			if got.FetchState.ConsecutiveFailures != 0 || got.FetchState.LastErrorAt != 0 || got.FetchState.LastError != "" {
				t.Fatalf("feed %d recorded false failure on shutdown: %+v", id, got.FetchState)
			}
		}
		if queuedHits.Load() != 0 {
			t.Fatalf("expected queued feed not to be requested after shutdown, got %d hits", queuedHits.Load())
		}
	})

	t.Run("caller parent deadline expiry after HTTP dispatch avoids failure backoff", func(t *testing.T) {
		dbPath := filepath.Join(t.TempDir(), "test.db")
		st, err := store.New(dbPath)
		if err != nil {
			t.Fatalf("create store: %v", err)
		}
		defer st.Close()

		dispatched := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			close(dispatched)
			<-r.Context().Done()
		}))
		defer server.Close()

		feed, err := st.CreateFeed(1, "Slow", server.URL+"/slow", "", "", nil)
		if err != nil {
			t.Fatalf("create feed: %v", err)
		}

		p := New(st, &config.Config{
			PullInterval:      1800,
			PullTimeout:       5,
			PullConcurrency:   1,
			PullMaxBackoff:    604800,
			AllowPrivateFeeds: true,
		})

		parentCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err = p.RefreshFeed(parentCtx, feed.ID)
		<-dispatched
		if err == nil {
			t.Fatal("expected parent deadline expiry error from RefreshFeed")
		}

		got, err := st.GetFeed(feed.ID)
		if err != nil {
			t.Fatalf("get feed: %v", err)
		}
		if got.FetchState.ConsecutiveFailures != 0 || got.FetchState.LastErrorAt != 0 || got.FetchState.LastError != "" {
			t.Fatalf("feed recorded false failure on parent deadline expiry: %+v", got.FetchState)
		}
	})
}
