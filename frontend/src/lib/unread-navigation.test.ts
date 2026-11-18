import { describe, expect, it } from "vitest";
import type { Feed, Group } from "@/lib/api";
import {
  canAdvanceFromReadScope,
  getScopeUnreadCount,
  resolveNextUnreadTarget,
} from "./unread-navigation";

function makeGroup(id: number, sortOrder = id): Group {
  return {
    id,
    name: `Group ${id}`,
    sort_order: sortOrder,
    created_at: 1000,
    updated_at: 1000,
  };
}

function makeFeed(id: number, groupId: number, unreadCount: number): Feed {
  return {
    id,
    group_id: groupId,
    name: `Feed ${id}`,
    link: `https://example.com/${id}.xml`,
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
      consecutive_failures: 0,
    },
    unread_count: unreadCount,
    item_count: unreadCount,
  };
}

describe("resolveNextUnreadTarget", () => {
  it("returns null for All group scope, Starred feed scope, or global scope", () => {
    const groups = [makeGroup(1), makeGroup(2)];
    const feeds = [makeFeed(10, 1, 3), makeFeed(20, 2, 5)];

    expect(
      resolveNextUnreadTarget({
        groups,
        beforeFeeds: feeds,
        afterFeeds: [makeFeed(10, 1, 0), makeFeed(20, 2, 5)],
        isFeedsSuccess: true,
        isFeedsError: false,
        selectedFeedId: null,
        selectedGroupId: 1,
        articleFilter: "all",
      }),
    ).toBeNull();

    expect(
      resolveNextUnreadTarget({
        groups,
        beforeFeeds: feeds,
        afterFeeds: [makeFeed(10, 1, 0), makeFeed(20, 2, 5)],
        isFeedsSuccess: true,
        isFeedsError: false,
        selectedFeedId: 10,
        selectedGroupId: null,
        articleFilter: "starred",
      }),
    ).toBeNull();

    expect(
      resolveNextUnreadTarget({
        groups,
        beforeFeeds: feeds,
        afterFeeds: [makeFeed(10, 1, 0), makeFeed(20, 2, 0)],
        isFeedsSuccess: true,
        isFeedsError: false,
        selectedFeedId: null,
        selectedGroupId: null,
        articleFilter: "unread",
      }),
    ).toBeNull();
  });

  it("advances from an already-zero feed scope", () => {
    const groups = [makeGroup(1), makeGroup(2)];
    const feeds = [makeFeed(10, 1, 0), makeFeed(20, 2, 3)];

    expect(
      resolveNextUnreadTarget({
        groups,
        beforeFeeds: feeds,
        afterFeeds: feeds,
        isFeedsSuccess: true,
        isFeedsError: false,
        selectedFeedId: 10,
        selectedGroupId: null,
        articleFilter: "unread",
      }),
    ).toEqual({ kind: "feed", feedId: 20 });
  });

  it.each(["unread", "all"] as const)(
    "advances from %s to the first unread feed in the same group",
    (articleFilter) => {
      const groups = [makeGroup(1), makeGroup(2)];
      const beforeFeeds = [
        makeFeed(10, 1, 0),
        makeFeed(11, 1, 4),
        makeFeed(12, 1, 2),
        makeFeed(13, 1, 5),
        makeFeed(21, 2, 7),
      ];
      const afterFeeds = beforeFeeds.map((f) =>
        f.id === 13 ? { ...f, unread_count: 0 } : f,
      );

      expect(
        resolveNextUnreadTarget({
          groups,
          beforeFeeds,
          afterFeeds,
          isFeedsSuccess: true,
          isFeedsError: false,
          selectedFeedId: 13,
          selectedGroupId: null,
          articleFilter,
        }),
      ).toEqual({ kind: "feed", feedId: 11 });
    },
  );

  it("crosses into the next folder when remaining feeds in current folder are read", () => {
    const groups = [makeGroup(1), makeGroup(2), makeGroup(3)];
    const beforeFeeds = [
      makeFeed(10, 1, 3),
      makeFeed(20, 2, 0),
      makeFeed(30, 3, 5),
    ];
    const afterFeeds = [
      makeFeed(10, 1, 0),
      makeFeed(20, 2, 0),
      makeFeed(30, 3, 5),
    ];

    expect(
      resolveNextUnreadTarget({
        groups,
        beforeFeeds,
        afterFeeds,
        isFeedsSuccess: true,
        isFeedsError: false,
        selectedFeedId: 10,
        selectedGroupId: null,
        articleFilter: "unread",
      }),
    ).toEqual({ kind: "feed", feedId: 30 });
  });

  it("wraps around to an earlier unread feed before falling back to all", () => {
    const groups = [makeGroup(1), makeGroup(2)];
    const beforeFeeds = [makeFeed(10, 1, 4), makeFeed(20, 2, 2)];
    const afterFeeds = [makeFeed(10, 1, 4), makeFeed(20, 2, 0)];

    expect(
      resolveNextUnreadTarget({
        groups,
        beforeFeeds,
        afterFeeds,
        isFeedsSuccess: true,
        isFeedsError: false,
        selectedFeedId: 20,
        selectedGroupId: null,
        articleFilter: "unread",
      }),
    ).toEqual({ kind: "feed", feedId: 10 });
  });

  it("falls back to ALL when no unread feeds remain anywhere", () => {
    const groups = [makeGroup(1), makeGroup(2)];
    const beforeFeeds = [makeFeed(10, 1, 4), makeFeed(20, 2, 0)];
    const afterFeeds = [makeFeed(10, 1, 0), makeFeed(20, 2, 0)];

    expect(
      resolveNextUnreadTarget({
        groups,
        beforeFeeds,
        afterFeeds,
        isFeedsSuccess: true,
        isFeedsError: false,
        selectedFeedId: 10,
        selectedGroupId: null,
        articleFilter: "unread",
      }),
    ).toEqual({ kind: "all" });
  });

  it("advances from an already-zero group scope", () => {
    const groups = [makeGroup(1), makeGroup(2)];
    const feeds = [makeFeed(10, 1, 0), makeFeed(20, 2, 4)];

    expect(
      resolveNextUnreadTarget({
        groups,
        beforeFeeds: feeds,
        afterFeeds: feeds,
        isFeedsSuccess: true,
        isFeedsError: false,
        selectedFeedId: null,
        selectedGroupId: 1,
        articleFilter: "unread",
      }),
    ).toEqual({ kind: "group", groupId: 2 });
  });

  it("advances folder-to-folder directly, wrapping around, then landing on ALL", () => {
    const groups = [makeGroup(1), makeGroup(2), makeGroup(3)];
    const beforeFeeds = [
      makeFeed(10, 1, 2),
      makeFeed(20, 2, 4),
      makeFeed(30, 3, 0),
    ];

    // Group 1 -> Group 2 (skipping Group 3 with 0 unread)
    expect(
      resolveNextUnreadTarget({
        groups,
        beforeFeeds,
        afterFeeds: [makeFeed(10, 1, 0), makeFeed(20, 2, 4), makeFeed(30, 3, 0)],
        isFeedsSuccess: true,
        isFeedsError: false,
        selectedFeedId: null,
        selectedGroupId: 1,
        articleFilter: "unread",
      }),
    ).toEqual({ kind: "group", groupId: 2 });

    // Group 2 -> wraps back to Group 1
    expect(
      resolveNextUnreadTarget({
        groups,
        beforeFeeds,
        afterFeeds: [makeFeed(10, 1, 2), makeFeed(20, 2, 0), makeFeed(30, 3, 0)],
        isFeedsSuccess: true,
        isFeedsError: false,
        selectedFeedId: null,
        selectedGroupId: 2,
        articleFilter: "unread",
      }),
    ).toEqual({ kind: "group", groupId: 1 });

    // No unread folders remaining -> ALL
    expect(
      resolveNextUnreadTarget({
        groups,
        beforeFeeds,
        afterFeeds: [makeFeed(10, 1, 0), makeFeed(20, 2, 0), makeFeed(30, 3, 0)],
        isFeedsSuccess: true,
        isFeedsError: false,
        selectedFeedId: null,
        selectedGroupId: 2,
        articleFilter: "unread",
      }),
    ).toEqual({ kind: "all" });
  });
});

describe("canAdvanceFromReadScope", () => {
  const base = {
    articleFilter: "unread" as const,
    selectedFeedId: 10,
    selectedGroupId: null,
    feedsReady: true,
    groupsReady: true,
    selectedFeedExists: true,
    selectedGroupExists: false,
    scopeUnreadCount: 0,
    loadedUnreadCount: 0,
  };

  it("enables navigation only for an existing scoped unread scope at zero", () => {
    expect(canAdvanceFromReadScope(base)).toBe(true);
    expect(
      canAdvanceFromReadScope({
        ...base,
        selectedFeedId: null,
        selectedGroupId: 1,
        selectedFeedExists: false,
        selectedGroupExists: true,
      }),
    ).toBe(true);
    expect(canAdvanceFromReadScope({ ...base, articleFilter: "all" })).toBe(
      false,
    );
    expect(canAdvanceFromReadScope({ ...base, articleFilter: "starred" })).toBe(
      false,
    );
    expect(
      canAdvanceFromReadScope({
        ...base,
        selectedFeedId: null,
        selectedGroupId: null,
      }),
    ).toBe(false);
  });

  it("rejects pending/failed metadata, missing scopes, and remaining unread articles", () => {
    expect(canAdvanceFromReadScope({ ...base, feedsReady: false })).toBe(false);
    expect(canAdvanceFromReadScope({ ...base, groupsReady: false })).toBe(
      false,
    );
    expect(
      canAdvanceFromReadScope({ ...base, selectedFeedExists: false }),
    ).toBe(false);
    expect(
      canAdvanceFromReadScope({ ...base, scopeUnreadCount: 1 }),
    ).toBe(false);
    expect(
      canAdvanceFromReadScope({ ...base, loadedUnreadCount: 1 }),
    ).toBe(false);
  });
});

describe("getScopeUnreadCount", () => {
  it("computes feed, group, and global unread totals", () => {
    const feeds = [makeFeed(1, 10, 3), makeFeed(2, 10, 4), makeFeed(3, 20, 5)];
    expect(getScopeUnreadCount(feeds, 1, null)).toBe(3);
    expect(getScopeUnreadCount(feeds, null, 10)).toBe(7);
    expect(getScopeUnreadCount(feeds, null, null)).toBe(12);
  });
});
