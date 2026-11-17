package store

import "testing"

func TestFeedContentSelectorsDefaultAndUpdate(t *testing.T) {
	store, _ := setupTestDB(t)
	defer closeStore(t, store)

	group := mustCreateGroup(t, store, "Selectors")
	feed := mustCreateFeed(t, store, group.ID, "Feed", "https://example.com/feed", "", "")

	if feed.ContentSelector != "" || feed.ExcludeSelector != "" {
		t.Fatalf("expected empty selector defaults, got %+v", feed)
	}

	contentSelector := "article .body"
	excludeSelector := ".ad, .related"
	if err := store.UpdateFeed(feed.ID, UpdateFeedParams{
		ContentSelector: &contentSelector,
		ExcludeSelector: &excludeSelector,
	}); err != nil {
		t.Fatalf("UpdateFeed() failed: %v", err)
	}

	updated, err := store.GetFeed(feed.ID)
	if err != nil {
		t.Fatalf("GetFeed() failed: %v", err)
	}
	if updated.ContentSelector != contentSelector || updated.ExcludeSelector != excludeSelector {
		t.Fatalf("selectors = %q / %q, want %q / %q",
			updated.ContentSelector, updated.ExcludeSelector, contentSelector, excludeSelector)
	}

	nextContentSelector := "main"
	if err := store.UpdateFeed(feed.ID, UpdateFeedParams{
		ContentSelector: &nextContentSelector,
	}); err != nil {
		t.Fatalf("UpdateFeed() partial failed: %v", err)
	}
	partial, err := store.GetFeed(feed.ID)
	if err != nil {
		t.Fatalf("GetFeed() failed: %v", err)
	}
	if partial.ContentSelector != nextContentSelector || partial.ExcludeSelector != excludeSelector {
		t.Fatalf("partial selectors = %q / %q, want %q / %q",
			partial.ContentSelector, partial.ExcludeSelector, nextContentSelector, excludeSelector)
	}

	empty := ""
	if err := store.UpdateFeed(feed.ID, UpdateFeedParams{
		ContentSelector: &empty,
		ExcludeSelector: &empty,
	}); err != nil {
		t.Fatalf("UpdateFeed() clear failed: %v", err)
	}
	cleared, err := store.GetFeed(feed.ID)
	if err != nil {
		t.Fatalf("GetFeed() failed: %v", err)
	}
	if cleared.ContentSelector != "" || cleared.ExcludeSelector != "" {
		t.Fatalf("expected cleared selectors, got %+v", cleared)
	}

	feeds, err := store.ListFeeds()
	if err != nil {
		t.Fatalf("ListFeeds() failed: %v", err)
	}
	if len(feeds) != 1 || feeds[0].ContentSelector != "" || feeds[0].ExcludeSelector != "" {
		t.Fatalf("ListFeeds() selectors = %+v", feeds)
	}
}
