package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
)

func TestListItems(t *testing.T) {
	store, _ := setupTestDB(t)
	defer closeStore(t, store)

	group := mustCreateGroup(t, store, "Test Group")
	feed1 := mustCreateFeed(t, store, group.ID, "Feed 1", "https://example.com/feed1", "https://example.com", "")
	feed2 := mustCreateFeed(t, store, group.ID, "Feed 2", "https://example.com/feed2", "https://example.com", "")

	// Create items with deterministic pub_date ordering.
	item1 := mustCreateItem(t, store, feed1.ID, "guid-1", "Item 1", "https://example.com/1", "Content 1", 100)
	item2 := mustCreateItem(t, store, feed1.ID, "guid-2", "Item 2", "https://example.com/2", "Content 2", 200)
	item3 := mustCreateItem(t, store, feed2.ID, "guid-3", "Item 3", "https://example.com/3", "Content 3", 300)

	// Mark item2 as read
	if err := store.UpdateItemUnread(item2.ID, false); err != nil {
		t.Fatalf("UpdateItemUnread() failed: %v", err)
	}

	t.Run("list all items", func(t *testing.T) {
		items, err := store.ListItems(ListItemsParams{})
		if err != nil {
			t.Fatalf("ListItems() failed: %v", err)
		}
		if len(items) != 3 {
			t.Errorf("expected 3 items, got %d", len(items))
		}
	})

	t.Run("filter by feed_id", func(t *testing.T) {
		items, err := store.ListItems(ListItemsParams{FeedID: &feed1.ID})
		if err != nil {
			t.Fatalf("ListItems() failed: %v", err)
		}
		if len(items) != 2 {
			t.Errorf("expected 2 items for feed1, got %d", len(items))
		}
	})

	t.Run("filter by unread=true", func(t *testing.T) {
		unread := true
		items, err := store.ListItems(ListItemsParams{Unread: &unread})
		if err != nil {
			t.Fatalf("ListItems() failed: %v", err)
		}
		if len(items) != 2 {
			t.Errorf("expected 2 unread items, got %d", len(items))
		}
	})

	t.Run("filter by unread=false", func(t *testing.T) {
		unread := false
		items, err := store.ListItems(ListItemsParams{Unread: &unread})
		if err != nil {
			t.Fatalf("ListItems() failed: %v", err)
		}
		if len(items) != 1 {
			t.Errorf("expected 1 read item, got %d", len(items))
		}
	})

	t.Run("pagination with limit and cursor", func(t *testing.T) {
		// First page: no cursor, returns the 2 newest items (item3 @300, item2 @200).
		page1, err := store.ListItems(ListItemsParams{Limit: 2})
		if err != nil {
			t.Fatalf("ListItems() failed: %v", err)
		}
		if len(page1) != 2 {
			t.Fatalf("expected 2 items on first page, got %d", len(page1))
		}
		if page1[0].ID != item3.ID || page1[1].ID != item2.ID {
			t.Errorf("expected first page [item3, item2], got ids [%d, %d]", page1[0].ID, page1[1].ID)
		}

		// Second page: cursor from the last item of page1 (item2).
		last := page1[len(page1)-1]
		page2, err := store.ListItems(ListItemsParams{
			Limit:         2,
			BeforePubDate: &last.PubDate,
			BeforeID:      &last.ID,
		})
		if err != nil {
			t.Fatalf("ListItems() failed: %v", err)
		}
		if len(page2) != 1 {
			t.Fatalf("expected 1 item on second page, got %d", len(page2))
		}
		if page2[0].ID != item1.ID {
			t.Errorf("expected second page [item1], got id %d", page2[0].ID)
		}

		// Beyond-last page: cursor from the final item returns nothing.
		last = page2[len(page2)-1]
		page3, err := store.ListItems(ListItemsParams{
			Limit:         2,
			BeforePubDate: &last.PubDate,
			BeforeID:      &last.ID,
		})
		if err != nil {
			t.Fatalf("ListItems() failed: %v", err)
		}
		if len(page3) != 0 {
			t.Errorf("expected empty page beyond last item, got %d", len(page3))
		}
	})

	t.Run("order by pub_date", func(t *testing.T) {
		items, err := store.ListItems(ListItemsParams{OrderBy: "pub_date"})
		if err != nil {
			t.Fatalf("ListItems() failed: %v", err)
		}
		if len(items) != 3 {
			t.Fatalf("expected 3 items, got %d", len(items))
		}
		if items[0].ID != item3.ID || items[1].ID != item2.ID || items[2].ID != item1.ID {
			t.Error("items not ordered by pub_date DESC")
		}
	})

	t.Run("stable order when pub_date ties", func(t *testing.T) {
		if _, err := store.db.Exec(
			`UPDATE items SET pub_date = :pub_date`,
			sql.Named("pub_date", int64(100)),
		); err != nil {
			t.Fatalf("failed to set pub_date for tie test: %v", err)
		}

		items, err := store.ListItems(ListItemsParams{OrderBy: "pub_date"})
		if err != nil {
			t.Fatalf("ListItems() failed: %v", err)
		}
		if len(items) != 3 {
			t.Fatalf("expected 3 items, got %d", len(items))
		}
		if items[0].ID != item3.ID || items[1].ID != item2.ID || items[2].ID != item1.ID {
			t.Error("items not ordered by pub_date DESC, id DESC")
		}
	})

	t.Run("order by created_at", func(t *testing.T) {
		if _, err := store.db.Exec(
			`UPDATE items SET created_at = :created_at WHERE id = :id`,
			sql.Named("created_at", int64(100)),
			sql.Named("id", item1.ID),
		); err != nil {
			t.Fatalf("failed to set created_at: %v", err)
		}
		if _, err := store.db.Exec(
			`UPDATE items SET created_at = :created_at WHERE id = :id`,
			sql.Named("created_at", int64(200)),
			sql.Named("id", item2.ID),
		); err != nil {
			t.Fatalf("failed to set created_at: %v", err)
		}
		if _, err := store.db.Exec(
			`UPDATE items SET created_at = :created_at WHERE id = :id`,
			sql.Named("created_at", int64(300)),
			sql.Named("id", item3.ID),
		); err != nil {
			t.Fatalf("failed to set created_at: %v", err)
		}

		items, err := store.ListItems(ListItemsParams{OrderBy: "created_at"})
		if err != nil {
			t.Fatalf("ListItems() failed: %v", err)
		}
		if len(items) != 3 {
			t.Fatalf("expected 3 items, got %d", len(items))
		}
		if items[0].ID != item3.ID || items[1].ID != item2.ID || items[2].ID != item1.ID {
			t.Error("items not ordered by created_at DESC")
		}
	})

	t.Run("stable order when created_at ties", func(t *testing.T) {
		if _, err := store.db.Exec(`UPDATE items SET created_at = :created_at`, sql.Named("created_at", int64(100))); err != nil {
			t.Fatalf("failed to set created_at for tie test: %v", err)
		}

		items, err := store.ListItems(ListItemsParams{OrderBy: "created_at"})
		if err != nil {
			t.Fatalf("ListItems() failed: %v", err)
		}
		if len(items) != 3 {
			t.Fatalf("expected 3 items, got %d", len(items))
		}
		if items[0].ID != item3.ID || items[1].ID != item2.ID || items[2].ID != item1.ID {
			t.Error("items not ordered by created_at DESC, id DESC")
		}
	})
}

func TestListItemsFilterByGroupID(t *testing.T) {
	store, _ := setupTestDB(t)
	defer closeStore(t, store)

	group1 := mustCreateGroup(t, store, "Group 1")
	group2 := mustCreateGroup(t, store, "Group 2")

	feed1 := mustCreateFeed(t, store, group1.ID, "Feed 1", "https://example.com/group1", "https://example.com", "")
	feed2 := mustCreateFeed(t, store, group2.ID, "Feed 2", "https://example.com/group2", "https://example.com", "")

	mustCreateItem(t, store, feed1.ID, "guid-1", "Item 1", "https://example.com/1", "Content 1", 100)
	mustCreateItem(t, store, feed2.ID, "guid-2", "Item 2", "https://example.com/2", "Content 2", 200)

	items, err := store.ListItems(ListItemsParams{GroupID: &group1.ID})
	if err != nil {
		t.Fatalf("ListItems() failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item for group1, got %d", len(items))
	}
	if items[0].FeedID != feed1.ID {
		t.Errorf("expected item to be from feed1, got feed_id=%d", items[0].FeedID)
	}
}

func TestGetItem(t *testing.T) {
	store, _ := setupTestDB(t)
	defer closeStore(t, store)

	group := mustCreateGroup(t, store, "Test Group")
	feed := mustCreateFeed(t, store, group.ID, "Test Feed", "https://example.com/feed", "https://example.com", "")

	created := mustCreateItem(t, store, feed.ID, "guid-1", "Test Item", "https://example.com/1", "Content", 123)

	item, err := store.GetItem(created.ID)
	if err != nil {
		t.Fatalf("GetItem() failed: %v", err)
	}

	if item.ID != created.ID || item.Title != created.Title {
		t.Error("retrieved item doesn't match created item")
	}

	_, err = store.GetItem(99999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for non-existent item, got %v", err)
	}
}

func TestCreateItem(t *testing.T) {
	store, _ := setupTestDB(t)
	defer closeStore(t, store)

	group := mustCreateGroup(t, store, "Test Group")
	feed := mustCreateFeed(t, store, group.ID, "Test Feed", "https://example.com/feed", "https://example.com", "")

	guid := "unique-guid-1"
	title := "Test Item"
	link := "https://example.com/item"
	content := "Test content"
	pubDate := int64(123)

	item := mustCreateItem(t, store, feed.ID, guid, title, link, content, pubDate)

	if item.GUID != guid || item.Title != title || item.Link != link || item.Content != content {
		t.Error("item fields don't match input")
	}

	if !item.Unread {
		t.Error("expected unread to default to true")
	}

	if item.ID == 0 || item.CreatedAt == 0 {
		t.Error("expected auto-populated fields to be set")
	}

	_, err := store.CreateItem(feed.ID, guid, "Different Title", link, content, pubDate)
	if err == nil {
		t.Error("expected error when creating duplicate feed_id+guid, got nil")
	}
}

func TestUpdateItemUnread(t *testing.T) {
	store, _ := setupTestDB(t)
	defer closeStore(t, store)

	group := mustCreateGroup(t, store, "Test Group")
	feed := mustCreateFeed(t, store, group.ID, "Test Feed", "https://example.com/feed", "https://example.com", "")
	item := mustCreateItem(t, store, feed.ID, "guid-1", "Test Item", "https://example.com/1", "Content", 123)

	if err := store.UpdateItemUnread(item.ID, false); err != nil {
		t.Fatalf("UpdateItemUnread() failed: %v", err)
	}

	updated, err := store.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem() failed: %v", err)
	}
	if updated.Unread {
		t.Error("expected unread to be false")
	}

	if err := store.UpdateItemUnread(item.ID, true); err != nil {
		t.Fatalf("UpdateItemUnread() failed: %v", err)
	}

	updated2, err := store.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem() failed: %v", err)
	}
	if !updated2.Unread {
		t.Error("expected unread to be true")
	}
}

func TestUpdateItemUnreadNotFound(t *testing.T) {
	store, _ := setupTestDB(t)
	defer closeStore(t, store)

	err := store.UpdateItemUnread(99999, false)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestBatchUpdateItemsUnread(t *testing.T) {
	store, _ := setupTestDB(t)
	defer closeStore(t, store)

	group := mustCreateGroup(t, store, "Test Group")
	feed := mustCreateFeed(t, store, group.ID, "Test Feed", "https://example.com/feed", "https://example.com", "")

	item1 := mustCreateItem(t, store, feed.ID, "guid-1", "Item 1", "https://example.com/1", "Content 1", 100)
	item2 := mustCreateItem(t, store, feed.ID, "guid-2", "Item 2", "https://example.com/2", "Content 2", 200)
	item3 := mustCreateItem(t, store, feed.ID, "guid-3", "Item 3", "https://example.com/3", "Content 3", 300)

	ids := []int64{item1.ID, item2.ID}
	if err := store.BatchUpdateItemsUnread(ids, false); err != nil {
		t.Fatalf("BatchUpdateItemsUnread() failed: %v", err)
	}

	updated1, err := store.GetItem(item1.ID)
	if err != nil {
		t.Fatalf("GetItem() failed: %v", err)
	}
	updated2, err := store.GetItem(item2.ID)
	if err != nil {
		t.Fatalf("GetItem() failed: %v", err)
	}
	updated3, err := store.GetItem(item3.ID)
	if err != nil {
		t.Fatalf("GetItem() failed: %v", err)
	}

	if updated1.Unread || updated2.Unread {
		t.Error("expected items 1 and 2 to be read")
	}
	if !updated3.Unread {
		t.Error("expected item 3 to remain unread")
	}

	if err := store.BatchUpdateItemsUnread([]int64{}, true); err != nil {
		t.Errorf("BatchUpdateItemsUnread() with empty ids should not error: %v", err)
	}
}

func TestBatchUpdateItemsUnreadChunked(t *testing.T) {
	store, _ := setupTestDB(t)
	defer closeStore(t, store)

	group := mustCreateGroup(t, store, "Chunk Group")
	feed := mustCreateFeed(t, store, group.ID, "Chunk Feed", "https://example.com/chunk-feed", "https://example.com", "")

	inputs := make([]BatchCreateItemInput, 0, 520)
	for i := range 520 {
		inputs = append(inputs, BatchCreateItemInput{
			GUID:    fmt.Sprintf("chunk-guid-%d", i),
			Title:   "Chunk Item",
			Link:    fmt.Sprintf("https://example.com/chunk/%d", i),
			Content: "Chunk Content",
			PubDate: int64(1000 + i),
		})
	}

	created, err := store.BatchCreateItemsIgnore(feed.ID, inputs)
	if err != nil {
		t.Fatalf("BatchCreateItemsIgnore() failed: %v", err)
	}
	if created != len(inputs) {
		t.Fatalf("expected %d created items, got %d", len(inputs), created)
	}

	items, err := store.ListItems(ListItemsParams{FeedID: &feed.ID})
	if err != nil {
		t.Fatalf("ListItems() failed: %v", err)
	}

	ids := make([]int64, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}

	if err := store.BatchUpdateItemsUnread(ids, false); err != nil {
		t.Fatalf("BatchUpdateItemsUnread() failed: %v", err)
	}

	unread := true
	unreadCount, err := store.CountItems(ListItemsParams{FeedID: &feed.ID, Unread: &unread})
	if err != nil {
		t.Fatalf("CountItems() failed: %v", err)
	}
	if unreadCount != 0 {
		t.Fatalf("expected 0 unread items, got %d", unreadCount)
	}
}

func TestMarkAllAsRead(t *testing.T) {
	store, _ := setupTestDB(t)
	defer closeStore(t, store)

	group := mustCreateGroup(t, store, "Test Group")
	feed1 := mustCreateFeed(t, store, group.ID, "Feed 1", "https://example.com/feed1", "https://example.com", "")
	feed2 := mustCreateFeed(t, store, group.ID, "Feed 2", "https://example.com/feed2", "https://example.com", "")

	item1 := mustCreateItem(t, store, feed1.ID, "guid-1", "Item 1", "https://example.com/1", "Content 1", 100)
	item2 := mustCreateItem(t, store, feed1.ID, "guid-2", "Item 2", "https://example.com/2", "Content 2", 200)
	item3 := mustCreateItem(t, store, feed2.ID, "guid-3", "Item 3", "https://example.com/3", "Content 3", 300)

	t.Run("mark all items in a specific feed", func(t *testing.T) {
		if err := store.MarkAllAsRead(&feed1.ID); err != nil {
			t.Fatalf("MarkAllAsRead() failed: %v", err)
		}

		updated1, err := store.GetItem(item1.ID)
		if err != nil {
			t.Fatalf("GetItem() failed: %v", err)
		}
		updated2, err := store.GetItem(item2.ID)
		if err != nil {
			t.Fatalf("GetItem() failed: %v", err)
		}
		updated3, err := store.GetItem(item3.ID)
		if err != nil {
			t.Fatalf("GetItem() failed: %v", err)
		}

		if updated1.Unread || updated2.Unread {
			t.Error("expected feed1 items to be read")
		}
		if !updated3.Unread {
			t.Error("expected feed2 items to remain unread")
		}
	})

	if err := store.UpdateItemUnread(item3.ID, true); err != nil {
		t.Fatalf("UpdateItemUnread() failed: %v", err)
	}

	t.Run("mark all items across all feeds", func(t *testing.T) {
		if err := store.MarkAllAsRead(nil); err != nil {
			t.Fatalf("MarkAllAsRead() failed: %v", err)
		}

		updated3, err := store.GetItem(item3.ID)
		if err != nil {
			t.Fatalf("GetItem() failed: %v", err)
		}
		if updated3.Unread {
			t.Error("expected all items to be read")
		}
	})
}

func TestItemExists(t *testing.T) {
	store, _ := setupTestDB(t)
	defer closeStore(t, store)

	group := mustCreateGroup(t, store, "Test Group")
	feed := mustCreateFeed(t, store, group.ID, "Test Feed", "https://example.com/feed", "https://example.com", "")

	guid := "test-guid"
	mustCreateItem(t, store, feed.ID, guid, "Test Item", "https://example.com/1", "Content", 123)

	exists, err := store.ItemExists(feed.ID, guid)
	if err != nil {
		t.Fatalf("ItemExists() failed: %v", err)
	}
	if !exists {
		t.Error("expected item to exist")
	}

	exists, err = store.ItemExists(feed.ID, "nonexistent-guid")
	if err != nil {
		t.Fatalf("ItemExists() failed: %v", err)
	}
	if exists {
		t.Error("expected item not to exist")
	}
}

func TestSearchItemsUsesFTS(t *testing.T) {
	store, _ := setupTestDB(t)
	defer closeStore(t, store)

	group := mustCreateGroup(t, store, "Search Group")
	feed := mustCreateFeed(t, store, group.ID, "Search Feed", "https://example.com/search.xml", "https://example.com", "")

	mustCreateItem(t, store, feed.ID, "s-1", "Go Concurrency", "https://example.com/a", "channels and goroutines", 100)
	item2 := mustCreateItem(t, store, feed.ID, "s-2", "Daily Notes", "https://example.com/b", "golang release", 200)

	results, err := store.SearchItems("gol", 10)
	if err != nil {
		t.Fatalf("SearchItems() failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected non-empty search results")
	}
	if results[0].ID != item2.ID {
		t.Fatalf("expected latest matching item first, got id=%d", results[0].ID)
	}
}

func TestSearchItemsFTSReflectsUpdateAndDelete(t *testing.T) {
	store, _ := setupTestDB(t)
	defer closeStore(t, store)

	group := mustCreateGroup(t, store, "FTS Group")
	feed := mustCreateFeed(t, store, group.ID, "FTS Feed", "https://example.com/fts.xml", "https://example.com", "")

	item := mustCreateItem(t, store, feed.ID, "fts-1", "Title", "https://example.com/fts", "alpha token", 100)

	results, err := store.SearchItems("alpha", 10)
	if err != nil {
		t.Fatalf("SearchItems() failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for alpha, got %d", len(results))
	}

	if _, err := store.db.Exec(`UPDATE items SET content = :content WHERE id = :id`, sql.Named("content", "beta token"), sql.Named("id", item.ID)); err != nil {
		t.Fatalf("update item content failed: %v", err)
	}

	results, err = store.SearchItems("alpha", 10)
	if err != nil {
		t.Fatalf("SearchItems() failed: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results for alpha after update, got %d", len(results))
	}

	results, err = store.SearchItems("beta", 10)
	if err != nil {
		t.Fatalf("SearchItems() failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for beta after update, got %d", len(results))
	}

	if _, err := store.db.Exec(`DELETE FROM items WHERE id = :id`, sql.Named("id", item.ID)); err != nil {
		t.Fatalf("delete item failed: %v", err)
	}

	results, err = store.SearchItems("beta", 10)
	if err != nil {
		t.Fatalf("SearchItems() failed: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results for beta after delete, got %d", len(results))
	}
}

func TestFTSUpdateTriggerSkipsUnreadAndNoopContentWrites(t *testing.T) {
	store, _ := setupTestDB(t)
	defer closeStore(t, store)

	group := mustCreateGroup(t, store, "FTS Trigger Group")
	feed := mustCreateFeed(t, store, group.ID, "FTS Feed", "https://example.com/trigger.xml", "https://example.com", "")
	item := mustCreateItem(t, store, feed.ID, "t-1", "Original Title", "https://example.com/t1", "alpha token", 100)

	countFTSData := func(t *testing.T) int {
		t.Helper()
		var n int
		if err := store.db.QueryRow(`SELECT COUNT(*) FROM items_fts_data`).Scan(&n); err != nil {
			t.Fatalf("count items_fts_data: %v", err)
		}
		return n
	}

	// Pin a single SQLite connection so total_changes() reflects writes on that connection accurately.
	conn, err := store.db.Conn(context.Background())
	if err != nil {
		t.Fatalf("acquire sqlite conn: %v", err)
	}
	defer conn.Close()

	beforeData := countFTSData(t)
	var beforeChanges, afterChanges int
	if err := conn.QueryRowContext(context.Background(), `SELECT total_changes()`).Scan(&beforeChanges); err != nil {
		t.Fatalf("read total_changes before unread update: %v", err)
	}

	// Read-state updates through store method and raw SQL must perform zero FTS writes.
	if err := store.UpdateItemUnread(item.ID, false); err != nil {
		t.Fatalf("UpdateItemUnread failed: %v", err)
	}
	if _, err := conn.ExecContext(context.Background(), `UPDATE items SET unread = 1 WHERE id = :id`, sql.Named("id", item.ID)); err != nil {
		t.Fatalf("raw unread update failed: %v", err)
	}
	// Identical title/content update must also skip FTS delete+insert.
	if _, err := conn.ExecContext(
		context.Background(),
		`UPDATE items SET title = :title, content = :content WHERE id = :id`,
		sql.Named("title", "Original Title"),
		sql.Named("content", "alpha token"),
		sql.Named("id", item.ID),
	); err != nil {
		t.Fatalf("noop title/content update failed: %v", err)
	}
	if err := conn.QueryRowContext(context.Background(), `SELECT total_changes()`).Scan(&afterChanges); err != nil {
		t.Fatalf("read total_changes after no-op updates: %v", err)
	}
	afterData := countFTSData(t)
	if afterData != beforeData {
		t.Fatalf("expected items_fts_data unchanged on unread/noop updates (%d -> %d)", beforeData, afterData)
	}
	// Only the two raw conn UPDATEs (unread=1 and identical title/content) should count on conn;
	// if the FTS trigger fired on either, FTS shadow writes would push delta > 2.
	if delta := afterChanges - beforeChanges; delta != 2 {
		t.Fatalf("expected total_changes delta = 2 (no FTS trigger writes), got %d", delta)
	}

	// Null -> non-null title change (and NULL <-> NULL no-op) must be handled null-safely.
	if _, err := conn.ExecContext(context.Background(), `UPDATE items SET title = NULL WHERE id = :id`, sql.Named("id", item.ID)); err != nil {
		t.Fatalf("set title NULL failed: %v", err)
	}
	beforeNullNoop := countFTSData(t)
	if _, err := conn.ExecContext(context.Background(), `UPDATE items SET title = NULL WHERE id = :id`, sql.Named("id", item.ID)); err != nil {
		t.Fatalf("noop NULL title update failed: %v", err)
	}
	if afterNullNoop := countFTSData(t); afterNullNoop != beforeNullNoop {
		t.Fatalf("expected NULL -> NULL title update to perform zero FTS writes (%d -> %d)", beforeNullNoop, afterNullNoop)
	}

	if _, err := conn.ExecContext(context.Background(), `UPDATE items SET title = 'UpdatedHeading' WHERE id = :id`, sql.Named("id", item.ID)); err != nil {
		t.Fatalf("NULL -> non-NULL title update failed: %v", err)
	}
	results, err := store.SearchItems("UpdatedHeading", 10)
	if err != nil {
		t.Fatalf("SearchItems after title change failed: %v", err)
	}
	if len(results) != 1 || results[0].ID != item.ID {
		t.Fatalf("expected updated title to be indexed in FTS, got %+v", results)
	}
}

func TestListItemsAndGetItemIncludeBookmarkID(t *testing.T) {
	store, _ := setupTestDB(t)
	defer closeStore(t, store)

	group := mustCreateGroup(t, store, "Bookmark ID Group")
	feed := mustCreateFeed(t, store, group.ID, "Feed", "https://example.com/rss", "https://example.com", "")
	item1 := mustCreateItem(t, store, feed.ID, "g1", "Item 1", "https://example.com/1", "c1", 200)
	item2 := mustCreateItem(t, store, feed.ID, "g2", "Item 2", "https://example.com/2", "c2", 100)

	b1, err := store.CreateBookmark(&item1.ID, &feed.ID, item1.Link, item1.Title, item1.Content, item1.PubDate, feed.Name)
	if err != nil {
		t.Fatalf("CreateBookmark: %v", err)
	}

	got1, err := store.GetItem(item1.ID)
	if err != nil {
		t.Fatalf("GetItem(item1): %v", err)
	}
	if got1.BookmarkID == nil || *got1.BookmarkID != b1.ID {
		t.Fatalf("GetItem(item1) BookmarkID = %v, want %d", got1.BookmarkID, b1.ID)
	}

	got2, err := store.GetItem(item2.ID)
	if err != nil {
		t.Fatalf("GetItem(item2): %v", err)
	}
	if got2.BookmarkID != nil {
		t.Fatalf("GetItem(item2) BookmarkID = %v, want nil", got2.BookmarkID)
	}

	// Simulate legacy duplicate item_id rows; scalar subquery must pick latest ID without row duplication.
	res, err := store.db.Exec(`
		INSERT INTO bookmarks (item_id, feed_id, link, title, content, pub_date, feed_name)
		VALUES (:item_id, :feed_id, 'https://example.com/1-legacy-dup', :title, :content, :pub_date, :feed_name)
	`,
		sql.Named("item_id", item1.ID),
		sql.Named("feed_id", feed.ID),
		sql.Named("title", item1.Title),
		sql.Named("content", item1.Content),
		sql.Named("pub_date", item1.PubDate),
		sql.Named("feed_name", feed.Name),
	)
	if err != nil {
		t.Fatalf("insert duplicate bookmark: %v", err)
	}
	dupID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}

	items, err := store.ListItems(ListItemsParams{FeedID: &feed.ID})
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items without duplicate join rows, got %d", len(items))
	}
	if items[0].ID != item1.ID || items[0].BookmarkID == nil || *items[0].BookmarkID != dupID {
		t.Fatalf("items[0] = (id=%d, bookmark_id=%v), want (id=%d, bookmark_id=%d)", items[0].ID, items[0].BookmarkID, item1.ID, dupID)
	}
	if items[1].ID != item2.ID || items[1].BookmarkID != nil {
		t.Fatalf("items[1] = (id=%d, bookmark_id=%v), want (id=%d, bookmark_id=nil)", items[1].ID, items[1].BookmarkID, item2.ID)
	}

	got1AfterDup, err := store.GetItem(item1.ID)
	if err != nil {
		t.Fatalf("GetItem(item1) after dup: %v", err)
	}
	if got1AfterDup.BookmarkID == nil || *got1AfterDup.BookmarkID != dupID {
		t.Fatalf("GetItem(item1) BookmarkID after dup = %v, want %d", got1AfterDup.BookmarkID, dupID)
	}

	if err := store.DeleteBookmark(dupID); err != nil {
		t.Fatalf("DeleteBookmark(dup): %v", err)
	}
	if err := store.DeleteBookmark(b1.ID); err != nil {
		t.Fatalf("DeleteBookmark(b1): %v", err)
	}

	got1Cleared, err := store.GetItem(item1.ID)
	if err != nil {
		t.Fatalf("GetItem(item1) after delete: %v", err)
	}
	if got1Cleared.BookmarkID != nil {
		t.Fatalf("GetItem(item1) BookmarkID after delete = %v, want nil", got1Cleared.BookmarkID)
	}
}
