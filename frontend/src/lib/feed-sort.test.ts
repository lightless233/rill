import { describe, expect, it } from "vitest";
import type { Feed, Group } from "@/lib/api";
import {
  moveGroupInVisibleList,
  reorderGroupsByDropIndex,
  sortFeedsUnreadFirst,
} from "./feed-sort";

function makeFeed(id: number, unreadCount: number): Feed {
  return {
    id,
    group_id: 2,
    name: `Feed ${id}`,
    link: `https://example.com/${id}.xml`,
    suspended: false,
    created_at: 0,
    updated_at: 0,
    fetch_state: {
      expires_at: 0,
      last_checked_at: 0,
      next_check_at: 0,
      last_http_status: 200,
      retry_after_until: 0,
      last_success_at: 0,
      last_error_at: 0,
      consecutive_failures: 0,
    },
    unread_count: unreadCount,
    item_count: 10,
  };
}

function makeGroup(id: number, sortOrder = id): Group {
  return {
    id,
    name: `Group ${id}`,
    sort_order: sortOrder,
    created_at: 0,
    updated_at: 0,
  };
}

describe("sortFeedsUnreadFirst", () => {
  it("moves unread feeds ahead of zero-unread feeds while preserving relative order", () => {
    const feeds = [
      makeFeed(1, 0),
      makeFeed(2, 3),
      makeFeed(3, 0),
      makeFeed(4, 1),
      makeFeed(5, 12),
    ];

    expect(sortFeedsUnreadFirst(feeds).map((f) => f.id)).toEqual([
      2, 4, 5, 1, 3,
    ]);
  });

  it("keeps single or already sorted lists intact", () => {
    const empty: Feed[] = [];
    expect(sortFeedsUnreadFirst(empty)).toEqual([]);

    const single = [makeFeed(1, 0)];
    expect(sortFeedsUnreadFirst(single)).toEqual(single);
  });
});

describe("reorderGroupsByDropIndex", () => {
  it("moves a group earlier or later while keeping hidden groups in their slots", () => {
    const allGroups = [makeGroup(1), makeGroup(2), makeGroup(3), makeGroup(4)];
    const visibleIds = [2, 3, 4]; // Group 1 (Default) is hidden.

    const moveUp = reorderGroupsByDropIndex(allGroups, visibleIds, 4, 0);
    expect(moveUp?.map((g) => g.id)).toEqual([1, 4, 2, 3]);
    expect(moveUp?.map((g) => g.sort_order)).toEqual([1, 2, 3, 4]);

    const moveDown = reorderGroupsByDropIndex(allGroups, visibleIds, 2, 3);
    expect(moveDown?.map((g) => g.id)).toEqual([1, 3, 4, 2]);
  });

  it("returns null for no-op drops onto the item's own surrounding slots", () => {
    const allGroups = [makeGroup(2), makeGroup(3), makeGroup(4)];
    const visibleIds = [2, 3, 4];

    expect(reorderGroupsByDropIndex(allGroups, visibleIds, 3, 1)).toBeNull();
    expect(reorderGroupsByDropIndex(allGroups, visibleIds, 3, 2)).toBeNull();
  });
});

describe("moveGroupInVisibleList", () => {
  it("shifts groups step-by-step and returns null at boundaries", () => {
    const allGroups = [makeGroup(1), makeGroup(2), makeGroup(3)];
    const visibleIds = [2, 3];

    expect(moveGroupInVisibleList(allGroups, visibleIds, 2, -1)).toBeNull();
    expect(moveGroupInVisibleList(allGroups, visibleIds, 3, 1)).toBeNull();

    const swapped = moveGroupInVisibleList(allGroups, visibleIds, 2, 1);
    expect(swapped?.map((g) => g.id)).toEqual([1, 3, 2]);
  });
});
