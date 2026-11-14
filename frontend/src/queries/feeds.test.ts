import { QueryClient } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";
import type { Feed } from "@/lib/api";
import { applyFeedCreatedCache } from "./feeds";
import { queryKeys } from "./keys";

function makeFeed(id: number): Feed {
  return {
    id,
    group_id: 1,
    name: `Feed ${id}`,
    link: `https://example.com/feed/${id}.xml`,
    site_url: "https://example.com",
    suspended: false,
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
    unread_count: 0,
    item_count: 0,
  };
}

describe("applyFeedCreatedCache", () => {
  it("appends created feed without duplication and invalidates feeds and items queries", () => {
    const qc = new QueryClient();
    const feedsListKey = queryKeys.feeds.list();
    const itemsListKey = [...queryKeys.items.lists(), { unread: true }, 20];

    qc.setQueryData(feedsListKey, [makeFeed(1)]);
    qc.setQueryData(itemsListKey, { pages: [], pageParams: [] });

    const created = makeFeed(2);
    applyFeedCreatedCache(qc, created);

    expect(qc.getQueryData<Feed[]>(feedsListKey)?.map((f) => f.id)).toEqual([
      1, 2,
    ]);
    expect(qc.getQueryState(feedsListKey)?.isInvalidated).toBe(true);
    expect(qc.getQueryState(itemsListKey)?.isInvalidated).toBe(true);
  });
});
