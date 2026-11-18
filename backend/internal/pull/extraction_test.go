package pull

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0x2E/fusion/internal/config"
	"github.com/0x2E/fusion/internal/model"
	"github.com/0x2E/fusion/internal/store"
)

func newExtractionPullTest(t *testing.T) (*store.Store, *model.Feed) {
	t.Helper()
	dbPath := t.TempDir() + "/test.db"
	st, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	group, err := st.CreateGroup("Extraction")
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	feed, err := st.CreateFeed(group.ID, "Feed", "https://example.com/feed", "", "", nil)
	if err != nil {
		t.Fatalf("create feed: %v", err)
	}
	return st, feed
}

func setSelectors(t *testing.T, st *store.Store, feedID int64, content, exclude string) {
	t.Helper()
	if err := st.UpdateFeed(feedID, store.UpdateFeedParams{
		ContentSelector: &content,
		ExcludeSelector: &exclude,
	}); err != nil {
		t.Fatalf("UpdateFeed() selectors failed: %v", err)
	}
}

func newExtractionPuller(st *store.Store) *Puller {
	return New(st, &config.Config{
		PullInterval:      1800,
		PullTimeout:       5,
		PullConcurrency:   1,
		PullMaxBackoff:    604800,
		AllowPrivateFeeds: true,
	})
}

func feedXML(feedURL string, items ...string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Demo</title><link>%s</link>%s</channel></rss>`,
		feedURL, strings.Join(items, ""))
}

func feedItem(guid, title, link, description string) string {
	return fmt.Sprintf(`<item><guid>%s</guid><title>%s</title><link>%s</link><description>%s</description></item>`,
		guid, title, link, description)
}

func articleServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var count atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	return server, &count
}

func TestRefreshFeedExtractsNewArticles(t *testing.T) {
	st, feed := newExtractionPullTest(t)
	setSelectors(t, st, feed.ID, "#body", "")

	articles, articleCalls := articleServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/one":
			_, _ = fmt.Fprint(w, `<html><body><div id="body">Extracted one</div></body></html>`)
		case "/two":
			_, _ = fmt.Fprint(w, `<html><body><div id="body">Extracted two</div></body></html>`)
		default:
			http.NotFound(w, r)
		}
	})

	secondURL := articles.URL + "/two"
	rss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprint(w, feedXML(articles.URL,
			feedItem("g1", "One", articles.URL+"/one", "RSS one"),
			fmt.Sprintf(`<item><guid>%s</guid><title>Two</title><description></description></item>`, secondURL),
		))
	}))
	defer rss.Close()

	if err := st.UpdateFeed(feed.ID, store.UpdateFeedParams{Link: &rss.URL}); err != nil {
		t.Fatalf("UpdateFeed() link failed: %v", err)
	}

	p := newExtractionPuller(st)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.RefreshFeed(ctx, feed.ID); err != nil {
		t.Fatalf("RefreshFeed() failed: %v", err)
	}

	items, err := st.ListItems(store.ListItemsParams{FeedID: &feed.ID})
	if err != nil {
		t.Fatalf("ListItems() failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	byGUID := make(map[string]string, len(items))
	for _, item := range items {
		byGUID[item.GUID] = item.Content
		if item.GUID == secondURL && item.Link != secondURL {
			t.Errorf("stored link = %q, want %q", item.Link, secondURL)
		}
	}
	if !strings.Contains(byGUID["g1"], "Extracted one") || !strings.Contains(byGUID[secondURL], "Extracted two") {
		t.Fatalf("expected extracted content, got %+v", byGUID)
	}
	if got := articleCalls.Load(); got != 2 {
		t.Fatalf("article requests = %d, want 2", got)
	}

	if err := p.RefreshFeed(ctx, feed.ID); err != nil {
		t.Fatalf("second RefreshFeed() failed: %v", err)
	}
	items, err = st.ListItems(store.ListItemsParams{FeedID: &feed.ID})
	if err != nil {
		t.Fatalf("ListItems() after second pull failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected no duplicate rows, got %d", len(items))
	}
	if got := articleCalls.Load(); got != 2 {
		t.Fatalf("article requests after second pull = %d, want 2", got)
	}
}

func TestRefreshFeedFallsBackToRSSOnExtractionFailure(t *testing.T) {
	st, feed := newExtractionPullTest(t)
	setSelectors(t, st, feed.ID, "#body", "")

	articles, articleCalls := articleServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusInternalServerError)
	})

	rss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprint(w, feedXML(articles.URL,
			feedItem("g1", "One", articles.URL+"/one", "RSS fallback"),
		))
	}))
	defer rss.Close()

	if err := st.UpdateFeed(feed.ID, store.UpdateFeedParams{Link: &rss.URL}); err != nil {
		t.Fatalf("UpdateFeed() link failed: %v", err)
	}

	p := newExtractionPuller(st)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.RefreshFeed(ctx, feed.ID); err != nil {
		t.Fatalf("RefreshFeed() failed: %v", err)
	}

	items, err := st.ListItems(store.ListItemsParams{FeedID: &feed.ID})
	if err != nil {
		t.Fatalf("ListItems() failed: %v", err)
	}
	if len(items) != 1 || items[0].Content != "RSS fallback" {
		t.Fatalf("expected RSS fallback, got %+v", items)
	}
	if got := articleCalls.Load(); got != 1 {
		t.Fatalf("article requests = %d, want 1", got)
	}

	updated, err := st.GetFeed(feed.ID)
	if err != nil {
		t.Fatalf("GetFeed() failed: %v", err)
	}
	if updated.FetchState.ConsecutiveFailures != 0 {
		t.Fatalf("extraction failure advanced source failures: %d", updated.FetchState.ConsecutiveFailures)
	}

	if err := p.RefreshFeed(ctx, feed.ID); err != nil {
		t.Fatalf("second RefreshFeed() failed: %v", err)
	}
	if got := articleCalls.Load(); got != 1 {
		t.Fatalf("stored fallback was retried: %d article requests", got)
	}
}

func TestRefreshFeedExtractionSkipsExistingAndDuplicateGUIDs(t *testing.T) {
	st, feed := newExtractionPullTest(t)
	setSelectors(t, st, feed.ID, "#body", "")

	if _, err := st.CreateItem(feed.ID, "existing", "Existing", "https://example.com/existing", "Old content", 1700000000); err != nil {
		t.Fatalf("CreateItem() failed: %v", err)
	}

	articles, articleCalls := articleServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body><div id="body">Extracted new</div></body></html>`)
	})

	rss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprint(w, feedXML(articles.URL,
			feedItem("existing", "Existing", articles.URL+"/existing", "RSS existing"),
			feedItem("new", "New", articles.URL+"/new", "RSS new"),
			feedItem("new", "New duplicate", articles.URL+"/new", "RSS duplicate"),
		))
	}))
	defer rss.Close()

	if err := st.UpdateFeed(feed.ID, store.UpdateFeedParams{Link: &rss.URL}); err != nil {
		t.Fatalf("UpdateFeed() link failed: %v", err)
	}

	p := newExtractionPuller(st)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.RefreshFeed(ctx, feed.ID); err != nil {
		t.Fatalf("RefreshFeed() failed: %v", err)
	}

	items, err := st.ListItems(store.ListItemsParams{FeedID: &feed.ID})
	if err != nil {
		t.Fatalf("ListItems() failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	byGUID := make(map[string]string, len(items))
	for _, item := range items {
		byGUID[item.GUID] = item.Content
	}
	if byGUID["existing"] != "Old content" {
		t.Fatalf("existing content was rewritten: %q", byGUID["existing"])
	}
	if !strings.Contains(byGUID["new"], "Extracted new") {
		t.Fatalf("new content = %q, want extracted", byGUID["new"])
	}
	if got := articleCalls.Load(); got != 1 {
		t.Fatalf("article requests = %d, want 1", got)
	}
}

func TestRefreshFeedExtractionStopsAtDeadline(t *testing.T) {
	st, feed := newExtractionPullTest(t)
	setSelectors(t, st, feed.ID, "#body", "")

	started := make(chan struct{}, 1)
	articles, articleCalls := articleServer(t, func(w http.ResponseWriter, _ *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		time.Sleep(300 * time.Millisecond)
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body><div id="body">Too late</div></body></html>`)
	})

	rss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprint(w, feedXML(articles.URL,
			feedItem("g1", "One", articles.URL+"/one", "RSS one"),
			feedItem("g2", "Two", articles.URL+"/two", "RSS two"),
			feedItem("g3", "Three", articles.URL+"/three", "RSS three"),
		))
	}))
	defer rss.Close()

	if err := st.UpdateFeed(feed.ID, store.UpdateFeedParams{Link: &rss.URL}); err != nil {
		t.Fatalf("UpdateFeed() link failed: %v", err)
	}

	p := newExtractionPuller(st)
	p.timeout = 150 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.RefreshFeed(ctx, feed.ID); err != nil {
		t.Fatalf("RefreshFeed() failed: %v", err)
	}

	items, err := st.ListItems(store.ListItemsParams{FeedID: &feed.ID})
	if err != nil {
		t.Fatalf("ListItems() failed: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected all 3 rows, got %d", len(items))
	}
	for _, item := range items {
		if !strings.HasPrefix(item.Content, "RSS ") {
			t.Fatalf("item %s content = %q, want RSS fallback", item.GUID, item.Content)
		}
	}
	if got := articleCalls.Load(); got != 1 {
		t.Fatalf("article requests = %d, want 1 after deadline", got)
	}

	updated, err := st.GetFeed(feed.ID)
	if err != nil {
		t.Fatalf("GetFeed() failed: %v", err)
	}
	if updated.FetchState.ConsecutiveFailures != 0 {
		t.Fatalf("deadline fallback advanced source failures: %d", updated.FetchState.ConsecutiveFailures)
	}
}

func TestRefreshFeedDisabledSelectorMakesNoArticleRequests(t *testing.T) {
	st, feed := newExtractionPullTest(t)

	articles, articleCalls := articleServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body><div id="body">Should not fetch</div></body></html>`)
	})

	rss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprint(w, feedXML(articles.URL,
			feedItem("g1", "One", articles.URL+"/one", "RSS only"),
		))
	}))
	defer rss.Close()

	if err := st.UpdateFeed(feed.ID, store.UpdateFeedParams{Link: &rss.URL}); err != nil {
		t.Fatalf("UpdateFeed() link failed: %v", err)
	}

	p := newExtractionPuller(st)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.RefreshFeed(ctx, feed.ID); err != nil {
		t.Fatalf("RefreshFeed() failed: %v", err)
	}

	items, err := st.ListItems(store.ListItemsParams{FeedID: &feed.ID})
	if err != nil {
		t.Fatalf("ListItems() failed: %v", err)
	}
	if len(items) != 1 || items[0].Content != "RSS only" {
		t.Fatalf("expected RSS content, got %+v", items)
	}
	if got := articleCalls.Load(); got != 0 {
		t.Fatalf("article requests = %d, want 0", got)
	}
}

func TestRefreshFeedExtractionCancellationStopsBeforeInsert(t *testing.T) {
	st, feed := newExtractionPullTest(t)
	setSelectors(t, st, feed.ID, "#body", "")

	started := make(chan struct{}, 1)
	articles, _ := articleServer(t, func(w http.ResponseWriter, _ *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		time.Sleep(500 * time.Millisecond)
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body><div id="body">Cancelled</div></body></html>`)
	})

	rss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprint(w, feedXML(articles.URL,
			feedItem("g1", "One", articles.URL+"/one", "RSS one"),
		))
	}))
	defer rss.Close()

	if err := st.UpdateFeed(feed.ID, store.UpdateFeedParams{Link: &rss.URL}); err != nil {
		t.Fatalf("UpdateFeed() link failed: %v", err)
	}

	p := newExtractionPuller(st)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- p.RefreshFeed(ctx, feed.ID)
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("article request did not start")
	}
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected cancellation error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RefreshFeed did not stop after cancellation")
	}

	items, err := st.ListItems(store.ListItemsParams{FeedID: &feed.ID})
	if err != nil {
		t.Fatalf("ListItems() failed: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("cancellation inserted %d items", len(items))
	}
}
