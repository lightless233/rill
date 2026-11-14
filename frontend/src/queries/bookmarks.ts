import { useMemo } from "react";
import {
  useInfiniteQuery,
  useMutation,
  useQueryClient,
  type InfiniteData,
  type QueryClient,
} from "@tanstack/react-query";
import {
  bookmarkAPI,
  type AppInfoResponse,
  type Bookmark,
  type Item,
  type PaginatedListResponse,
} from "@/lib/api";
import {
  normalizeBookmarkFilters,
  queryKeys,
  type BookmarkFilters,
  type NormalizedBookmarkFilters,
} from "./keys";
import type { ItemsInfiniteData } from "./items";
import { usePreferencesStore } from "@/store";

type BookmarkListResponse = PaginatedListResponse<Bookmark>;
export type BookmarksInfiniteData = InfiniteData<BookmarkListResponse, string | null>;

export function resolveBookmarkItemId(bookmark: Bookmark): number {
  return bookmark.item_id ?? -bookmark.id;
}

function buildListBookmarksParams(
  filters: NormalizedBookmarkFilters,
  cursor: string | null,
  pageSize: number,
) {
  const params: Parameters<typeof bookmarkAPI.list>[0] = {
    limit: pageSize,
  };
  if (filters.feedId) params.feed_id = filters.feedId;
  if (filters.groupId) params.group_id = filters.groupId;
  if (cursor) params.before = cursor;
  return params;
}

// useBookmarks is the shared infinite query over bookmarks. Callers pass the
// filters and page size that fit their use case (lookup vs. starred list).
function useBookmarks(
  filters: BookmarkFilters,
  pageSize: number,
  enabled = true,
) {
  const normalized = normalizeBookmarkFilters(filters);
  return useInfiniteQuery({
    queryKey: [...queryKeys.bookmarks.lists(), normalized, pageSize],
    queryFn: async ({ pageParam }) =>
      bookmarkAPI.list(
        buildListBookmarksParams(normalized, pageParam, pageSize),
      ),
    initialPageParam: null as string | null,
    getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
    staleTime: Number.POSITIVE_INFINITY,
    enabled,
  });
}

export function updateItemsBookmarkCache(
  qc: QueryClient,
  params: {
    itemId?: number | null;
    matchBookmarkId?: number;
    nextBookmarkId: number | null;
  },
) {
  const { itemId, matchBookmarkId, nextBookmarkId } = params;
  const matchesItem = (item: Item) =>
    (itemId != null && itemId > 0 && item.id === itemId) ||
    (matchBookmarkId != null && item.bookmark_id === matchBookmarkId);

  qc.setQueriesData<ItemsInfiniteData>(
    { queryKey: queryKeys.items.lists() },
    (old) => {
      if (!old) return old;
      let changed = false;
      const pages = old.pages.map((page) => {
        let pageChanged = false;
        const data = page.data.map((item) => {
          if (matchesItem(item) && item.bookmark_id !== nextBookmarkId) {
            pageChanged = true;
            return { ...item, bookmark_id: nextBookmarkId };
          }
          return item;
        });
        if (!pageChanged) return page;
        changed = true;
        return { ...page, data };
      });
      return changed ? { ...old, pages } : old;
    },
  );

  if (itemId != null && itemId > 0) {
    qc.setQueryData<Item>(queryKeys.items.detail(itemId), (old) =>
      old && old.bookmark_id !== nextBookmarkId
        ? { ...old, bookmark_id: nextBookmarkId }
        : old,
    );
  }
}

export interface StarredItemsResult {
  items: Item[];
  bookmarks: Bookmark[];
  hasNextPage: boolean;
  isLoading: boolean;
  isFetchingNextPage: boolean;
  isError: boolean;
  isFetchNextPageError: boolean;
  error: Error | null;
  refetch: () => void;
  retry: () => void;
  fetchNextPage: () => void;
}

// useStarredItems is the paginated, server-filtered starred list. Filtering by
// feed/group is pushed to the backend, so pagination is correct in every scope.
export function useStarredItems(
  filters: BookmarkFilters,
  enabled = true,
): StarredItemsResult {
  const pageSize = usePreferencesStore((state) => state.articlePageSize);
  const query = useBookmarks(filters, pageSize, enabled);

  const bookmarks = useMemo(
    () => query.data?.pages.flatMap((p) => p.data) ?? [],
    [query.data],
  );

  const items = useMemo<Item[]>(
    () =>
      bookmarks.map((bookmark) => ({
        id: bookmark.item_id ?? -bookmark.id,
        feed_id: bookmark.feed_id ?? 0,
        guid: bookmark.link || `bookmark:${bookmark.id}`,
        title: bookmark.title,
        link: bookmark.link,
        content: bookmark.content,
        pub_date: bookmark.pub_date,
        unread: bookmark.unread,
        created_at: bookmark.created_at,
        bookmark_id: bookmark.id,
      })),
    [bookmarks],
  );

  return {
    items,
    bookmarks,
    hasNextPage: query.hasNextPage,
    isLoading: query.isLoading,
    isFetchingNextPage: query.isFetchingNextPage,
    isError: query.isError,
    isFetchNextPageError: query.isFetchNextPageError,
    error: query.error,
    refetch: () => void query.refetch(),
    retry: () =>
      void (query.isFetchNextPageError
        ? query.fetchNextPage()
        : query.refetch()),
    fetchNextPage: () => void query.fetchNextPage(),
  };
}

export function useCreateBookmark() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: async (item: Item) => {
      // item_id mode: the backend snapshots link/title/content/feed_name
      // from the item itself, which is always fresher than client copies.
      const res = await bookmarkAPI.create({ item_id: item.id });
      return res.data!;
    },
    onSuccess: (bookmark, item) => {
      const itemId = resolveBookmarkItemId(bookmark);
      const wasAlreadyStarred =
        item.bookmark_id === bookmark.id || item.bookmark_id != null;

      updateItemsBookmarkCache(qc, {
        itemId: bookmark.item_id ?? item.id,
        nextBookmarkId: bookmark.id,
      });

      qc.setQueriesData<BookmarksInfiniteData>(
        { queryKey: queryKeys.bookmarks.lists() },
        (old) => {
          if (!old) return old;
          const pages = [...old.pages];
          const first = pages[0];
          if (!first) return old;

          const index = first.data.findIndex(
            (b) => resolveBookmarkItemId(b) === itemId,
          );
          if (index !== -1) {
            const newData = [...first.data];
            newData[index] = bookmark;
            pages[0] = { ...first, data: newData };
          } else {
            pages[0] = {
              ...first,
              data: [bookmark, ...first.data],
              total: first.total + 1,
            };
          }
          return { ...old, pages };
        },
      );

      if (!wasAlreadyStarred) {
        qc.setQueryData<AppInfoResponse>(queryKeys.app.info(), (old) =>
          old ? { ...old, bookmark_count: old.bookmark_count + 1 } : old,
        );
      }

      qc.invalidateQueries({ queryKey: queryKeys.bookmarks.all });
      qc.invalidateQueries({ queryKey: queryKeys.items.all });
      qc.invalidateQueries({ queryKey: queryKeys.app.info() });
    },
  });
}

export interface DeleteBookmarkVariables {
  bookmarkId: number;
  itemId?: number | null;
}

export function useDeleteBookmark() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: async ({ bookmarkId }: DeleteBookmarkVariables) => {
      await bookmarkAPI.delete(bookmarkId);
      return bookmarkId;
    },
    onSuccess: (bookmarkId, { itemId }) => {
      updateItemsBookmarkCache(qc, {
        itemId,
        matchBookmarkId: bookmarkId,
        nextBookmarkId: null,
      });

      qc.setQueriesData<BookmarksInfiniteData>(
        { queryKey: queryKeys.bookmarks.lists() },
        (old) => {
          if (!old) return old;
          const pages = old.pages.map((page) => {
            const newData = page.data.filter((b) => b.id !== bookmarkId);
            if (newData.length === page.data.length) return page;
            return {
              ...page,
              data: newData,
              total: Math.max(0, page.total - 1),
            };
          });
          return { ...old, pages };
        },
      );

      qc.setQueryData<AppInfoResponse>(queryKeys.app.info(), (old) =>
        old
          ? { ...old, bookmark_count: Math.max(0, old.bookmark_count - 1) }
          : old,
      );

      qc.invalidateQueries({ queryKey: queryKeys.bookmarks.all });
      qc.invalidateQueries({ queryKey: queryKeys.items.all });
      qc.invalidateQueries({ queryKey: queryKeys.app.info() });
    },
  });
}
