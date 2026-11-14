import { QueryClient } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";
import type { Item } from "@/lib/api";
import { queryKeys } from "./keys";
import { updateItemsBookmarkCache } from "./bookmarks";
import type { ItemsInfiniteData } from "./items";

function makeItem(id: number, bookmarkId: number | null): Item {
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
    bookmark_id: bookmarkId,
  };
}

describe("updateItemsBookmarkCache", () => {
  it("sets and clears bookmark_id across list pages and item details including >100 bookmark ids", () => {
    const qc = new QueryClient();
    const listKey = [...queryKeys.items.lists(), { feedId: null }, 20];
    const detailKey = queryKeys.items.detail(2);

    const initialList: ItemsInfiniteData = {
      pages: [
        {
          data: [makeItem(1, 5), makeItem(2, null)],
          total: 2,
          next_cursor: null,
        },
      ],
      pageParams: [null],
    };

    qc.setQueryData(listKey, initialList);
    qc.setQueryData(detailKey, makeItem(2, null));

    // Simulate starring item 2 when global bookmarks already exceed 100 (bookmark id = 155).
    updateItemsBookmarkCache(qc, { itemId: 2, nextBookmarkId: 155 });

    const afterStarList = qc.getQueryData<ItemsInfiniteData>(listKey);
    expect(afterStarList?.pages[0].data.map((i) => i.bookmark_id)).toEqual([
      5, 155,
    ]);
    expect(qc.getQueryData<Item>(detailKey)?.bookmark_id).toBe(155);

    // Unstarring item 2 clears bookmark_id back to null everywhere while preserving item 1.
    updateItemsBookmarkCache(qc, {
      itemId: 2,
      matchBookmarkId: 155,
      nextBookmarkId: null,
    });

    const afterUnstarList = qc.getQueryData<ItemsInfiniteData>(listKey);
    expect(afterUnstarList?.pages[0].data.map((i) => i.bookmark_id)).toEqual([
      5, null,
    ]);
    expect(qc.getQueryData<Item>(detailKey)?.bookmark_id).toBeNull();
  });
});
