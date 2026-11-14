import { useCallback, useMemo } from "react";
import { useItems } from "@/queries/items";
import {
  resolveBookmarkItemId,
  useStarredItems,
} from "@/queries/bookmarks";
import { useArticlePinsStore, articleListKey } from "@/store";
import type { Bookmark, Item } from "@/lib/api";
import type { ArticleFilter } from "@/lib/article-filter";

interface ArticleListFilters {
  feedId: number | null;
  groupId: number | null;
  articleFilter: ArticleFilter;
}

export function useArticleList(filters: ArticleListFilters) {
  const isStarredMode = filters.articleFilter === "starred";
  const isUnreadMode = filters.articleFilter === "unread";

  // Items are unused in starred mode (bookmarks ARE the articles there), so
  // skip the request entirely.
  const itemsQuery = useItems(
    {
      feedId: filters.feedId,
      groupId: filters.groupId,
      unread: isUnreadMode ? true : undefined,
    },
    !isStarredMode,
  );

  // Server-filtered, paginated starred list. Only fetched in starred mode.
  const starred = useStarredItems(
    { feedId: filters.feedId, groupId: filters.groupId },
    isStarredMode,
  );

  // Pinned ids for this exact list keep just-marked-read rows visible in the
  // unread view so the marking can be undone in place (see article-pins.ts).
  const pinsKey = articleListKey(
    filters.feedId,
    filters.groupId,
    filters.articleFilter,
  );
  const pinnedIds = useArticlePinsStore((s) => s.pins[pinsKey]);

  const items = useMemo(
    () => itemsQuery.data?.pages.flatMap((p) => p.data) ?? [],
    [itemsQuery.data],
  );

  const visibleItems = useMemo(() => {
    if (!isUnreadMode) return items;

    // The unread view always shows exactly `unread || pinned`: with no pins
    // left (none written yet, or cleared on leaving the list) rows already
    // marked read must drop out of the cache-driven display.
    return items.filter(
      (item) => item.unread || (pinnedIds?.has(item.id) ?? false),
    );
  }, [items, isUnreadMode, pinnedIds]);

  const articles: Item[] = isStarredMode ? starred.items : visibleItems;

  // Bookmark metadata for orphaned starred items (e.g. feed_name snapshot).
  // Non-starred items carry bookmark_id directly from GET /api/items, avoiding preloading.
  const bookmarkByItemId = useMemo(
    () =>
      isStarredMode
        ? new Map<number, Bookmark>(
            starred.bookmarks.map((b) => [resolveBookmarkItemId(b), b]),
          )
        : new Map<number, Bookmark>(),
    [isStarredMode, starred.bookmarks],
  );

  const getBookmarkByItemId = useCallback(
    (itemId: number) => bookmarkByItemId.get(itemId),
    [bookmarkByItemId],
  );

  const isError = isStarredMode ? starred.isError : itemsQuery.isError;
  const isFetchNextPageError = isStarredMode
    ? starred.isFetchNextPageError
    : itemsQuery.isFetchNextPageError;
  const error = isStarredMode ? starred.error : itemsQuery.error;
  const refetch = isStarredMode
    ? starred.refetch
    : () => void itemsQuery.refetch();
  const retry = isStarredMode
    ? starred.retry
    : () =>
        void (itemsQuery.isFetchNextPageError
          ? itemsQuery.fetchNextPage()
          : itemsQuery.refetch());

  return {
    articles,
    hasMore: isStarredMode ? starred.hasNextPage : itemsQuery.hasNextPage,
    isLoading: isStarredMode ? starred.isLoading : itemsQuery.isLoading,
    isLoadingMore: isStarredMode
      ? starred.isFetchingNextPage
      : itemsQuery.isFetchingNextPage,
    isError,
    isFetchNextPageError,
    error,
    refetch,
    retry,
    isStarredMode,
    fetchNextPage: isStarredMode
      ? starred.fetchNextPage
      : () => void itemsQuery.fetchNextPage(),
    getBookmarkByItemId,
  };
}
