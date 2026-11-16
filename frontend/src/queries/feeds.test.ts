import { QueryClient } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { feedAPI, type Feed, type ListResponse } from "@/lib/api";
import {
  applyFeedCreatedCache,
  feedQueries,
  refreshFeedsQuery,
} from "./feeds";
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

describe("refreshFeedsQuery", () => {
  it("creates and fetches the feeds query when no observer has mounted yet", async () => {
    const qc = new QueryClient();
    const spy = vi
      .spyOn(feedAPI, "list")
      .mockResolvedValue({ data: [makeFeed(1)], total: 1 });

    try {
      await refreshFeedsQuery(qc);
      expect(qc.getQueryData<Feed[]>(queryKeys.feeds.list())?.map((f) => f.id)).toEqual([
        1,
      ]);
      expect(spy).toHaveBeenCalledTimes(1);
    } finally {
      spy.mockRestore();
    }
  });

  it("cancels a first in-flight feeds request instead of reusing it", async () => {
    const qc = new QueryClient();
    const responses: Array<(value: ListResponse<Feed>) => void> = [];
    const spy = vi.spyOn(feedAPI, "list").mockImplementation(
      () =>
        new Promise<ListResponse<Feed>>((resolve) => {
          responses.push(resolve);
        }),
    );

    try {
      const first = qc.fetchQuery(feedQueries.list()).catch(() => undefined);
      for (let i = 0; i < 20 && responses.length < 1; i += 1) {
        await new Promise((resolve) => setTimeout(resolve, 0));
      }
      expect(responses.length).toBe(1);

      const refresh = refreshFeedsQuery(qc);
      for (let i = 0; i < 20 && responses.length < 2; i += 1) {
        await new Promise((resolve) => setTimeout(resolve, 0));
      }
      expect(responses.length).toBe(2);

      responses[1]({ data: [makeFeed(2)], total: 1 });
      await refresh;
      expect(
        qc.getQueryData<Feed[]>(queryKeys.feeds.list())?.map((f) => f.id),
      ).toEqual([2]);

      // A late response from the canceled request must not overwrite the
      // notification-triggered result.
      responses[0]({ data: [makeFeed(1)], total: 1 });
      await first;
      expect(
        qc.getQueryData<Feed[]>(queryKeys.feeds.list())?.map((f) => f.id),
      ).toEqual([2]);
    } finally {
      spy.mockRestore();
    }
  });
});

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
