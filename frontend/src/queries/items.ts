import {
  infiniteQueryOptions,
  queryOptions,
  type QueryClient,
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
  type InfiniteData,
} from "@tanstack/react-query";
import {
  itemAPI,
  type Feed,
  type Item,
  type ListItemsParams,
  type MarkAllItemsReadRequest,
} from "@/lib/api";
import {
  normalizeItemFilters,
  queryKeys,
  type ItemFilters,
  type NormalizedItemFilters,
} from "./keys";
import { usePreferencesStore } from "@/store";
import type { BookmarksInfiniteData } from "./bookmarks";

type ItemListResponse = Awaited<ReturnType<typeof itemAPI.list>>;
export type ItemsInfiniteData = InfiniteData<ItemListResponse, string | null>;

// Mirrors the backend's maxBatchUpdateIDs for PATCH /items/-/read and -/unread.
const MAX_BATCH_IDS = 1000;

// Bumped whenever a whole-scope mark-all-read succeeds. A read-state mutation
// started earlier carries the older epoch, so its late failure must not roll
// rows back to unread over the newer server-wide result.
const markAllEpochs = new WeakMap<QueryClient, number>();

function getMarkAllEpoch(qc: QueryClient) {
  return markAllEpochs.get(qc) ?? 0;
}

export type ItemsMutationContext = {
  markAllEpoch: number;
  targetUnread: boolean;
  prevItemUnreadById: Map<number, boolean>;
  prevBookmarkUnreadByItemId: Map<number, boolean>;
  appliedFeedDeltas: Map<number, number>;
};

function buildListItemsParams(
  filters: NormalizedItemFilters,
  cursor: string | null,
  pageSize: number,
): ListItemsParams {
  const params: ListItemsParams = {
    limit: pageSize,
    order_by: "pub_date",
  };

  if (filters.feedId) params.feed_id = filters.feedId;
  if (filters.groupId) params.group_id = filters.groupId;
  if (filters.unread) params.unread = true;
  if (cursor) params.before = cursor;

  return params;
}

export const itemQueries = {
  list: (filters: ItemFilters, pageSize: number) => {
    const normalizedFilters = normalizeItemFilters(filters);

    return infiniteQueryOptions({
      queryKey: [...queryKeys.items.lists(), normalizedFilters, pageSize],
      queryFn: async ({ pageParam }) =>
        itemAPI.list(buildListItemsParams(normalizedFilters, pageParam, pageSize)),
      initialPageParam: null as string | null,
      getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
    });
  },
  detail: (itemId: number) =>
    queryOptions({
      queryKey: queryKeys.items.detail(itemId),
      queryFn: async () => {
        const res = await itemAPI.get(itemId);
        return res.data;
      },
    }),
};

export function useItems(filters: ItemFilters, enabled = true) {
  const articlePageSize = usePreferencesStore((state) => state.articlePageSize);
  return useInfiniteQuery({ ...itemQueries.list(filters, articlePageSize), enabled });
}

export function useItem(itemId: number | null, enabled = true) {
  return useQuery({
    ...itemQueries.detail(itemId ?? 0),
    enabled: enabled && itemId !== null && itemId > 0,
  });
}

export function beginItemsReadStateOptimisticUpdate(
  qc: QueryClient,
  ids: number[],
  targetUnread: boolean,
): ItemsMutationContext {
  const idSet = new Set(ids);
  const desiredFeedDeltas = new Map<number, number>();
  const countedFeedItemIds = new Set<number>();
  const updatedItemsById = new Map<number, Item>();
  const prevItemUnreadById = new Map<number, boolean>();
  const prevBookmarkUnreadByItemId = new Map<number, boolean>();
  const appliedFeedDeltas = new Map<number, number>();

  qc.setQueriesData<ItemsInfiniteData>(
    { queryKey: queryKeys.items.lists() },
    (old) => {
      if (!old) return old;

      let changed = false;
      const pages = old.pages.map((page) => {
        let pageChanged = false;
        const data = page.data.map((item) => {
          if (!idSet.has(item.id) || item.unread === targetUnread) {
            return item;
          }

          if (!prevItemUnreadById.has(item.id)) {
            prevItemUnreadById.set(item.id, item.unread);
          }
          if (!countedFeedItemIds.has(item.id)) {
            countedFeedItemIds.add(item.id);
            const delta = targetUnread ? 1 : -1;
            desiredFeedDeltas.set(
              item.feed_id,
              (desiredFeedDeltas.get(item.feed_id) ?? 0) + delta,
            );
          }

          const updatedItem = { ...item, unread: targetUnread };
          updatedItemsById.set(item.id, updatedItem);
          pageChanged = true;
          return updatedItem;
        });

        if (!pageChanged) return page;
        changed = true;
        return { ...page, data };
      });

      return changed ? { ...old, pages } : old;
    },
  );

  for (const id of ids) {
    const optimisticItem = updatedItemsById.get(id);
    qc.setQueryData<Item>(queryKeys.items.detail(id), (old) => {
      if (!old) {
        return optimisticItem;
      }
      if (old.unread === targetUnread) {
        return old;
      }

      if (!prevItemUnreadById.has(id)) {
        prevItemUnreadById.set(id, old.unread);
      }
      if (!countedFeedItemIds.has(id)) {
        countedFeedItemIds.add(id);
        const delta = targetUnread ? 1 : -1;
        desiredFeedDeltas.set(
          old.feed_id,
          (desiredFeedDeltas.get(old.feed_id) ?? 0) + delta,
        );
      }
      return { ...old, unread: targetUnread };
    });
  }

  // Mirror the read-state change into bookmark caches so the starred view (which
  // renders bookmarks as articles) reflects the toggle without a refetch.
  qc.setQueriesData<BookmarksInfiniteData>(
    { queryKey: queryKeys.bookmarks.lists() },
    (old) => {
      if (!old) return old;
      let changed = false;
      const pages = old.pages.map((page) => {
        let pageChanged = false;
        const newData = page.data.map((bookmark) => {
          if (
            bookmark.item_id != null &&
            idSet.has(bookmark.item_id) &&
            bookmark.unread !== targetUnread
          ) {
            if (!prevBookmarkUnreadByItemId.has(bookmark.item_id)) {
              prevBookmarkUnreadByItemId.set(bookmark.item_id, bookmark.unread);
            }
            if (!prevItemUnreadById.has(bookmark.item_id)) {
              prevItemUnreadById.set(bookmark.item_id, bookmark.unread);
            }
            if (
              bookmark.feed_id != null &&
              !countedFeedItemIds.has(bookmark.item_id)
            ) {
              countedFeedItemIds.add(bookmark.item_id);
              const delta = targetUnread ? 1 : -1;
              desiredFeedDeltas.set(
                bookmark.feed_id,
                (desiredFeedDeltas.get(bookmark.feed_id) ?? 0) + delta,
              );
            }
            pageChanged = true;
            return { ...bookmark, unread: targetUnread };
          }
          return bookmark;
        });
        if (!pageChanged) return page;
        changed = true;
        return { ...page, data: newData };
      });
      return changed ? { ...old, pages } : old;
    },
  );

  if (desiredFeedDeltas.size > 0) {
    qc.setQueryData(queryKeys.feeds.list(), (old: Feed[] | undefined) =>
      old?.map((feed) => {
        const delta = desiredFeedDeltas.get(feed.id) ?? 0;
        if (delta === 0) return feed;

        const nextUnread = Math.max(0, feed.unread_count + delta);
        appliedFeedDeltas.set(feed.id, nextUnread - feed.unread_count);
        return {
          ...feed,
          unread_count: nextUnread,
        };
      }),
    );
  }

  return {
    markAllEpoch: getMarkAllEpoch(qc),
    targetUnread,
    prevItemUnreadById,
    prevBookmarkUnreadByItemId,
    appliedFeedDeltas,
  };
}

export function rollbackItemsReadStateMutation(
  qc: QueryClient,
  context: ItemsMutationContext | undefined,
) {
  if (!context || context.markAllEpoch !== getMarkAllEpoch(qc)) return;

  // Surgical field-level rollback preserves concurrent bookmark_id / list edits
  // committed while the read-state request was still in flight.
  if (context.prevItemUnreadById.size > 0) {
    qc.setQueriesData<ItemsInfiniteData>(
      { queryKey: queryKeys.items.lists() },
      (old) => {
        if (!old) return old;
        let changed = false;
        const pages = old.pages.map((page) => {
          let pageChanged = false;
          const data = page.data.map((item) => {
            const prevUnread = context.prevItemUnreadById.get(item.id);
            if (
              prevUnread === undefined ||
              item.unread !== context.targetUnread
            ) {
              return item;
            }
            pageChanged = true;
            return { ...item, unread: prevUnread };
          });
          if (!pageChanged) return page;
          changed = true;
          return { ...page, data };
        });
        return changed ? { ...old, pages } : old;
      },
    );

    for (const [id, prevUnread] of context.prevItemUnreadById) {
      qc.setQueryData<Item>(queryKeys.items.detail(id), (old) => {
        if (!old || old.unread !== context.targetUnread) {
          return old;
        }
        return { ...old, unread: prevUnread };
      });
    }
  }

  if (context.appliedFeedDeltas.size > 0) {
    qc.setQueryData(queryKeys.feeds.list(), (old: Feed[] | undefined) =>
      old?.map((feed) => {
        const applied = context.appliedFeedDeltas.get(feed.id) ?? 0;
        if (applied === 0) return feed;

        return {
          ...feed,
          unread_count: Math.max(0, feed.unread_count - applied),
        };
      }),
    );
  }

  if (
    context.prevBookmarkUnreadByItemId.size > 0 ||
    context.prevItemUnreadById.size > 0
  ) {
    qc.setQueriesData<BookmarksInfiniteData>(
      { queryKey: queryKeys.bookmarks.lists() },
      (old) => {
        if (!old) return old;
        let changed = false;
        const pages = old.pages.map((page) => {
          let pageChanged = false;
          const newData = page.data.map((bookmark) => {
            if (bookmark.item_id == null) return bookmark;
            const prevUnread =
              context.prevBookmarkUnreadByItemId.get(bookmark.item_id) ??
              context.prevItemUnreadById.get(bookmark.item_id);
            if (
              prevUnread === undefined ||
              bookmark.unread !== context.targetUnread
            ) {
              return bookmark;
            }
            pageChanged = true;
            return { ...bookmark, unread: prevUnread };
          });
          if (!pageChanged) return page;
          changed = true;
          return { ...page, data: newData };
        });
        return changed ? { ...old, pages } : old;
      },
    );
  }
}

function useSetItemsReadState(targetUnread: boolean) {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: async (ids: number[]) => {
      // Sequential chunks: a failure stops the rest, leaving earlier chunks
      // applied on the server (reconciled in onError).
      for (let i = 0; i < ids.length; i += MAX_BATCH_IDS) {
        const chunk = ids.slice(i, i + MAX_BATCH_IDS);
        if (targetUnread) {
          await itemAPI.markUnread({ ids: chunk });
        } else {
          await itemAPI.markRead({ ids: chunk });
        }
      }

      return ids;
    },
    onMutate: async (ids) => {
      await Promise.all([
        qc.cancelQueries({ queryKey: queryKeys.items.all }),
        qc.cancelQueries({ queryKey: queryKeys.feeds.all }),
      ]);

      return beginItemsReadStateOptimisticUpdate(qc, ids, targetUnread);
    },
    onError: async (_error, ids, context) => {
      rollbackItemsReadStateMutation(qc, context);
      // A multi-chunk failure may have been partially applied server-side, so
      // the blanket rollback can disagree with the server; refetch to settle.
      if (ids.length > MAX_BATCH_IDS) {
        await qc.invalidateQueries({ queryKey: queryKeys.items.all });
      }
    },
    onSettled: async () => {
      await qc.invalidateQueries({ queryKey: queryKeys.feeds.all });
    },
  });
}

export function buildMarkAllItemsReadPayload(
  selectedFeedId: number | null,
  selectedGroupId: number | null,
): MarkAllItemsReadRequest {
  if (selectedFeedId !== null) {
    return { feed_id: selectedFeedId };
  }
  if (selectedGroupId !== null) {
    return { group_id: selectedGroupId };
  }
  return {};
}

function patchItemListsUnread(
  qc: QueryClient,
  transform: (item: Item) => boolean | undefined,
) {
  qc.setQueriesData<ItemsInfiniteData>(
    { queryKey: queryKeys.items.lists() },
    (old) => {
      if (!old) return old;
      let changed = false;
      const pages = old.pages.map((page) => {
        let pageChanged = false;
        const data = page.data.map((item) => {
          const nextUnread = transform(item);
          if (nextUnread === undefined || item.unread === nextUnread) {
            return item;
          }
          pageChanged = true;
          return { ...item, unread: nextUnread };
        });
        if (!pageChanged) return page;
        changed = true;
        return { ...page, data };
      });
      return changed ? { ...old, pages } : old;
    },
  );
}

function patchBookmarkListsUnread(
  qc: QueryClient,
  transform: (bookmark: BookmarksInfiniteData["pages"][number]["data"][number]) => boolean | undefined,
) {
  qc.setQueriesData<BookmarksInfiniteData>(
    { queryKey: queryKeys.bookmarks.lists() },
    (old) => {
      if (!old) return old;
      let changed = false;
      const pages = old.pages.map((page) => {
        let pageChanged = false;
        const data = page.data.map((bookmark) => {
          const nextUnread = transform(bookmark);
          if (nextUnread === undefined || bookmark.unread === nextUnread) {
            return bookmark;
          }
          pageChanged = true;
          return { ...bookmark, unread: nextUnread };
        });
        if (!pageChanged) return page;
        changed = true;
        return { ...page, data };
      });
      return changed ? { ...old, pages } : old;
    },
  );
}

export function applyScopeMarkAllReadCache(
  qc: QueryClient,
  scope: MarkAllItemsReadRequest,
  affectedIds: number[],
) {
  markAllEpochs.set(qc, getMarkAllEpoch(qc) + 1);

  qc.setQueryData(queryKeys.feeds.list(), (old: Feed[] | undefined) =>
    old?.map((feed) => {
      const matches =
        scope.feed_id !== undefined
          ? feed.id === scope.feed_id
          : scope.group_id !== undefined
            ? feed.group_id === scope.group_id
            : true;
      if (!matches || feed.unread_count === 0) return feed;
      return { ...feed, unread_count: 0 };
    }),
  );

  if (affectedIds.length === 0) {
    return;
  }

  const idSet = new Set(affectedIds);
  patchItemListsUnread(qc, (item) => (idSet.has(item.id) ? false : undefined));
  for (const id of affectedIds) {
    qc.setQueryData<Item>(queryKeys.items.detail(id), (old) =>
      old && old.unread ? { ...old, unread: false } : old,
    );
  }
  patchBookmarkListsUnread(qc, (bookmark) =>
    bookmark.item_id != null && idSet.has(bookmark.item_id) ? false : undefined,
  );
}

export async function invalidateAfterMarkAllRead(qc: QueryClient) {
  markAllEpochs.set(qc, getMarkAllEpoch(qc) + 1);
  await Promise.all([
    qc.invalidateQueries({ queryKey: queryKeys.items.all }),
    qc.invalidateQueries({ queryKey: queryKeys.feeds.all }),
    qc.invalidateQueries({ queryKey: queryKeys.bookmarks.lists() }),
  ]);
}

export function useMarkItemsRead() {
  return useSetItemsReadState(false);
}

export function useMarkItemsUnread() {
  return useSetItemsReadState(true);
}

export function useMarkAllItemsRead() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: async (data: MarkAllItemsReadRequest) => {
      const res = await itemAPI.markAllRead(data);
      return res.data?.affected_ids ?? [];
    },
    onSuccess: (affectedIds, scope) => {
      applyScopeMarkAllReadCache(qc, scope, affectedIds);
      void invalidateAfterMarkAllRead(qc);
    },
  });
}
