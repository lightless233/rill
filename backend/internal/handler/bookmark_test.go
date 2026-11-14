package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/0x2E/fusion/internal/model"
)

func TestListBookmarksCursorPagination(t *testing.T) {
	h, st := newFeverTestHandler(t)

	group, err := st.CreateGroup("G")
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	feed, err := st.CreateFeed(group.ID, "Feed", "https://example.com/feed", "https://example.com", "", nil)
	if err != nil {
		t.Fatalf("CreateFeed: %v", err)
	}
	// created_at is non-decreasing across inserts and id is auto-increment, so
	// ORDER BY created_at DESC, id DESC is deterministic regardless of second
	// resolution: the page order is always [b3, b2, b1].
	b1, err := st.CreateBookmark(nil, &feed.ID, "https://example.com/1", "Bookmark 1", "c", 100, feed.Name)
	if err != nil {
		t.Fatalf("CreateBookmark 1: %v", err)
	}
	b2, err := st.CreateBookmark(nil, &feed.ID, "https://example.com/2", "Bookmark 2", "c", 100, feed.Name)
	if err != nil {
		t.Fatalf("CreateBookmark 2: %v", err)
	}
	b3, err := st.CreateBookmark(nil, &feed.ID, "https://example.com/3", "Bookmark 3", "c", 100, feed.Name)
	if err != nil {
		t.Fatalf("CreateBookmark 3: %v", err)
	}

	r := newTestRouter()
	r.GET("/api/bookmarks", h.listBookmarks)

	// First page: no cursor, expect the 2 newest bookmarks and a non-null next_cursor.
	w := performRequest(r, http.MethodGet, "/api/bookmarks?limit=2", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	var page struct {
		Data       []model.Bookmark `json:"data"`
		Total      int              `json:"total"`
		NextCursor *string          `json:"next_cursor"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(page.Data) != 2 {
		t.Fatalf("expected 2 bookmarks on first page, got %d", len(page.Data))
	}
	if page.Data[0].ID != b3.ID || page.Data[1].ID != b2.ID {
		t.Errorf("expected first page [b3, b2], got ids [%d, %d]", page.Data[0].ID, page.Data[1].ID)
	}
	if page.Total != 3 {
		t.Errorf("expected total=3, got %d", page.Total)
	}
	if page.NextCursor == nil {
		t.Fatal("expected non-null next_cursor on a full first page")
	}
	last := page.Data[len(page.Data)-1]
	wantCursor := fmt.Sprintf("%d_%d", last.CreatedAt, last.ID)
	if *page.NextCursor != wantCursor {
		t.Errorf("expected next_cursor %q, got %q", wantCursor, *page.NextCursor)
	}

	// Second page: use the cursor, expect the remaining bookmark and a null next_cursor.
	w = performRequest(r, http.MethodGet, "/api/bookmarks?limit=2&before="+*page.NextCursor, nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	page = struct {
		Data       []model.Bookmark `json:"data"`
		Total      int              `json:"total"`
		NextCursor *string          `json:"next_cursor"`
	}{}
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(page.Data) != 1 || page.Data[0].ID != b1.ID {
		t.Errorf("expected second page [b1], got %+v", page.Data)
	}
	if page.NextCursor != nil {
		t.Errorf("expected null next_cursor on final page, got %q", *page.NextCursor)
	}
}

func TestListBookmarksCursorInvalidBefore(t *testing.T) {
	h, _ := newFeverTestHandler(t)

	r := newTestRouter()
	r.GET("/api/bookmarks", h.listBookmarks)

	w := performRequest(r, http.MethodGet, "/api/bookmarks?before=not-a-cursor", nil, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for malformed cursor, got %d", w.Code)
	}
}

func TestGetAppInfoIncludesBookmarkCountAndItemBookmarkID(t *testing.T) {
	h, st := newAnonTestHandler(t, &recordingPuller{})
	defer st.Close()

	r := h.SetupRouter()

	readAppBookmarkCount := func() int {
		t.Helper()
		w := performRequest(r, http.MethodGet, "/api/app", nil, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("GET /api/app status = %d, body = %s", w.Code, w.Body.String())
		}
		var resp struct {
			Data appInfoResponse `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode /api/app: %v", err)
		}
		return resp.Data.BookmarkCount
	}

	if got := readAppBookmarkCount(); got != 0 {
		t.Fatalf("initial bookmark_count = %d, want 0", got)
	}

	group, err := st.CreateGroup("Test")
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	feed, err := st.CreateFeed(group.ID, "Feed", "https://example.com/rss", "https://example.com", "", nil)
	if err != nil {
		t.Fatalf("CreateFeed: %v", err)
	}
	item, err := st.CreateItem(feed.ID, "guid-1", "Item 1", "https://example.com/1", "content", 1000)
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}

	// Verify item endpoints serialize explicit null before starring.
	w := performRequest(r, http.MethodGet, fmt.Sprintf("/api/items/%d", item.ID), nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/items/%d status = %d", item.ID, w.Code)
	}
	var itemResp struct {
		Data model.Item `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &itemResp); err != nil {
		t.Fatalf("decode /api/items/:id: %v", err)
	}
	if itemResp.Data.BookmarkID != nil {
		t.Fatalf("expected bookmark_id nil before starring, got %v", *itemResp.Data.BookmarkID)
	}

	// Create 105 bookmarks including one tied to the item to confirm count goes past 100.
	firstBookmark, err := st.CreateBookmark(&item.ID, &feed.ID, item.Link, item.Title, item.Content, item.PubDate, feed.Name)
	if err != nil {
		t.Fatalf("CreateBookmark: %v", err)
	}
	for i := 1; i < 105; i++ {
		if _, err := st.CreateBookmark(
			nil,
			&feed.ID,
			fmt.Sprintf("https://example.com/extra-%d", i),
			fmt.Sprintf("Extra %d", i),
			"content",
			int64(1000+i),
			feed.Name,
		); err != nil {
			t.Fatalf("CreateBookmark(%d): %v", i, err)
		}
	}

	if got := readAppBookmarkCount(); got != 105 {
		t.Fatalf("bookmark_count = %d, want 105", got)
	}

	w = performRequest(r, http.MethodGet, "/api/items", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/items status = %d", w.Code)
	}
	var listResp struct {
		Data []model.Item `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode /api/items: %v", err)
	}
	if len(listResp.Data) != 1 || listResp.Data[0].BookmarkID == nil || *listResp.Data[0].BookmarkID != firstBookmark.ID {
		t.Fatalf("expected item bookmark_id = %d in /api/items, got %+v", firstBookmark.ID, listResp.Data)
	}

	if err := st.DeleteBookmark(firstBookmark.ID); err != nil {
		t.Fatalf("DeleteBookmark: %v", err)
	}
	if got := readAppBookmarkCount(); got != 104 {
		t.Fatalf("bookmark_count after delete = %d, want 104", got)
	}

	// Confirm CountBookmarks failures return 500 rather than fabricating 0.
	if err := st.Close(); err != nil {
		t.Fatalf("Close store: %v", err)
	}
	w = performRequest(r, http.MethodGet, "/api/app", nil, nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("GET /api/app with closed store = %d, want 500", w.Code)
	}
}
