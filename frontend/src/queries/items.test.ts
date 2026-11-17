import { QueryClient } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";
import type { Bookmark, Feed, Item } from "@/lib/api";
import { updateItemsBookmarkCache, type BookmarksInfiniteData } from "./bookmarks";
import {
  applyScopeMarkAllReadCache,
  beginItemsReadStateOptimisticUpdate,
  buildMarkAllItemsReadPayload,
  invalidateAfterMarkAllRead,
  rollbackItemsReadStateMutation,
  type ItemsInfiniteData,
} from "./items";
import { queryKeys } from "./keys";

function makeItem(id: number, overrides: Partial<Item> = {}): Item {
  return {
    id,
    feed_id: 1,
    guid: `guid-${id}`,
    title: `Title ${id}`,
    link: `https://example.com/${id}`,
    content: "body",
    pub_date: 1000 + id,
    unread: true,
    created_at: 1000 + id,
    bookmark_id: null,
    ...overrides,
  };
}

function makeFeed(id: number, unreadCount: number): Feed {
  return {
    id,
    group_id: 1,
    name: `Feed ${id}`,
    link: `https://example.com/feed/${id}.xml`,
    site_url: "https://example.com",
    suspended: false,
    content_selector: "",
    exclude_selector: "",
    created_at: 1000,
    updated_at: 1000,
    fetch_state: {
      expires_at: 0,
      last_checked_at: 0,
      next_check_at: 0,
      last_http_status: 200,
      retry_after_until: 0,
      last_success_at: 0,
      last_error_at: 0,
      last_error: "",
      consecutive_failures: 0,
    },
    unread_count: unreadCount,
    item_count: unreadCount,
  };
}

function makeBookmark(id: number, itemId: number, unread: boolean): Bookmark {
  return {
    id,
    item_id: itemId,
    feed_id: 1,
    link: `https://example.com/${itemId}`,
    title: `Title ${itemId}`,
    content: "body",
    pub_date: 1000 + itemId,
    feed_name: "Feed 1",
    unread,
    created_at: 2000 + id,
  };
}

describe("items read-state rollback", () => {
  it("restores unread state on failure without clobbering concurrent star/unstar edits", () => {
    const qc = new QueryClient();
    const listKey = [...queryKeys.items.lists(), { feedId: null }, 20];
    const detailKey1 = queryKeys.items.detail(1);
    const detailKey2 = queryKeys.items.detail(2);
    const bookmarkListKey = [...queryKeys.bookmarks.lists(), { feedId: null }, 20];

    const item1 = makeItem(1, { unread: true, bookmark_id: null });
    const item2 = makeItem(2, { unread: true, bookmark_id: 55 });

    const listData: ItemsInfiniteData = {
      pages: [{ data: [item1, item2], total: 2, next_cursor: null }],
      pageParams: [null],
    };
    const bookmarkData: BookmarksInfiniteData = {
      pages: [
        {
          data: [makeBookmark(55, 2, true)],
          total: 1,
          next_cursor: null,
        },
      ],
      pageParams: [null],
    };

    qc.setQueryData(listKey, listData);
    qc.setQueryData(detailKey1, item1);
    qc.setQueryData(detailKey2, item2);
    qc.setQueryData(queryKeys.feeds.list(), [makeFeed(1, 5)]);
    qc.setQueryData(bookmarkListKey, bookmarkData);

    // 1. Mark [1, 2] as read optimistically.
    const ctx = beginItemsReadStateOptimisticUpdate(qc, [1, 2], false);
    expect(qc.getQueryData<Feed[]>(queryKeys.feeds.list())?.[0].unread_count).toBe(3);

    // 2. While mark-read is in flight: star item 1 (bookmark 99) and unstar item 2.
    updateItemsBookmarkCache(qc, { itemId: 1, nextBookmarkId: 99 });
    updateItemsBookmarkCache(qc, {
      itemId: 2,
      matchBookmarkId: 55,
      nextBookmarkId: null,
    });

    // 3. Mark-read fails and rolls back.
    rollbackItemsReadStateMutation(qc, ctx);

    const rolledList = qc.getQueryData<ItemsInfiniteData>(listKey);
    expect(rolledList?.pages[0].data[0]).toMatchObject({
      id: 1,
      unread: true,
      bookmark_id: 99,
    });
    expect(rolledList?.pages[0].data[1]).toMatchObject({
      id: 2,
      unread: true,
      bookmark_id: null,
    });

    expect(qc.getQueryData<Item>(detailKey1)).toMatchObject({
      unread: true,
      bookmark_id: 99,
    });
    expect(qc.getQueryData<Item>(detailKey2)).toMatchObject({
      unread: true,
      bookmark_id: null,
    });
    expect(qc.getQueryData<Feed[]>(queryKeys.feeds.list())?.[0].unread_count).toBe(5);
    expect(
      qc.getQueryData<BookmarksInfiniteData>(bookmarkListKey)?.pages[0].data[0]
        .unread,
    ).toBe(true);
  });

  it("deduplicates unread_count deltas when an item is cached in multiple item lists", () => {
    const qc = new QueryClient();
    const listAllKey = [...queryKeys.items.lists(), { unread: false }, 20];
    const listUnreadKey = [...queryKeys.items.lists(), { unread: true }, 20];
    const item1 = makeItem(1, { unread: true });

    qc.setQueryData<ItemsInfiniteData>(listAllKey, {
      pages: [{ data: [item1], total: 1, next_cursor: null }],
      pageParams: [null],
    });
    qc.setQueryData<ItemsInfiniteData>(listUnreadKey, {
      pages: [{ data: [item1], total: 1, next_cursor: null }],
      pageParams: [null],
    });
    qc.setQueryData(queryKeys.feeds.list(), [makeFeed(1, 2)]);

    const ctx = beginItemsReadStateOptimisticUpdate(qc, [1], false);
    expect(qc.getQueryData<Feed[]>(queryKeys.feeds.list())?.[0].unread_count).toBe(1);

    rollbackItemsReadStateMutation(qc, ctx);
    expect(qc.getQueryData<Feed[]>(queryKeys.feeds.list())?.[0].unread_count).toBe(2);
  });
});

describe("mark all items read helpers", () => {
  it("skips a stale rollback that started before a successful mark-all", async () => {
    const qc = new QueryClient();
    const listKey = [...queryKeys.items.lists(), { unread: true }, 20];
    qc.setQueryData<ItemsInfiniteData>(listKey, {
      pages: [{ data: [makeItem(1, { unread: true })], total: 1, next_cursor: null }],
      pageParams: [null],
    });
    qc.setQueryData(queryKeys.feeds.list(), [makeFeed(1, 1)]);

    const ctx = beginItemsReadStateOptimisticUpdate(qc, [1], false);
    await invalidateAfterMarkAllRead(qc);
    // Server-wide read state is now authoritative; simulate its refetched data.
    qc.setQueryData(queryKeys.feeds.list(), [makeFeed(1, 0)]);

    rollbackItemsReadStateMutation(qc, ctx);

    expect(qc.getQueryData<ItemsInfiniteData>(listKey)?.pages[0].data[0].unread).toBe(false);
    expect(qc.getQueryData<Feed[]>(queryKeys.feeds.list())?.[0].unread_count).toBe(0);
  });

  it("builds mutually exclusive scope payloads", () => {
    expect(buildMarkAllItemsReadPayload(12, 3)).toEqual({ feed_id: 12 });
    expect(buildMarkAllItemsReadPayload(null, 3)).toEqual({ group_id: 3 });
    expect(buildMarkAllItemsReadPayload(null, null)).toEqual({});
  });

  it("invalidates items, feeds, and bookmark lists after mark-all-read", async () => {
    const qc = new QueryClient();
    const listKey = [...queryKeys.items.lists(), { unread: true }, 20];
    const detailKey = queryKeys.items.detail(1);
    const feedsKey = queryKeys.feeds.list();
    const bookmarksKey = [...queryKeys.bookmarks.lists(), {}, 20];

    qc.setQueryData(listKey, { pages: [], pageParams: [] });
    qc.setQueryData(detailKey, makeItem(1));
    qc.setQueryData(feedsKey, [makeFeed(1, 5)]);
    qc.setQueryData(bookmarksKey, { pages: [], pageParams: [] });

    await invalidateAfterMarkAllRead(qc);

    expect(qc.getQueryState(listKey)?.isInvalidated).toBe(true);
    expect(qc.getQueryState(detailKey)?.isInvalidated).toBe(true);
    expect(qc.getQueryState(feedsKey)?.isInvalidated).toBe(true);
    expect(qc.getQueryState(bookmarksKey)?.isInvalidated).toBe(true);
  });

  it("updates feeds unread counts and affected item caches synchronously for mark-all-read", () => {
    const qc = new QueryClient();
    const listKey = [...queryKeys.items.lists(), { unread: true }, 20];
    const feedsKey = queryKeys.feeds.list();
    const bookmarksKey = [...queryKeys.bookmarks.lists(), {}, 20];

    qc.setQueryData<ItemsInfiniteData>(listKey, {
      pages: [
        {
          data: [makeItem(1, { feed_id: 1, unread: true }), makeItem(2, { feed_id: 2, unread: true })],
          total: 2,
          next_cursor: null,
        },
      ],
      pageParams: [null],
    });
    qc.setQueryData(feedsKey, [makeFeed(1, 5), { ...makeFeed(2, 4), group_id: 2 }]);
    qc.setQueryData<BookmarksInfiniteData>(bookmarksKey, {
      pages: [{ data: [makeBookmark(10, 1, true)], total: 1, next_cursor: null }],
      pageParams: [null],
    });

    applyScopeMarkAllReadCache(qc, { feed_id: 1 }, [1]);

    const feeds = qc.getQueryData<Feed[]>(feedsKey)!;
    expect(feeds[0].unread_count).toBe(0);
    expect(feeds[1].unread_count).toBe(4);
    expect(qc.getQueryData<ItemsInfiniteData>(listKey)?.pages[0].data[0].unread).toBe(false);
    expect(qc.getQueryData<ItemsInfiniteData>(listKey)?.pages[0].data[1].unread).toBe(true);
    expect(qc.getQueryData<BookmarksInfiniteData>(bookmarksKey)?.pages[0].data[0].unread).toBe(false);
  });
});
