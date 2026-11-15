import { useCallback, useEffect, useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { CheckCheck, ChevronDown, Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ArticleItem } from "./article-item";
import { ContentHeader } from "@/components/layout/content-header";
import { SidebarTrigger } from "@/components/layout/sidebar-trigger";
import { useArticleNavigation } from "@/hooks/use-keyboard";
import { useUrlState, type ArticleFilter } from "@/hooks/use-url-state";
import { useArticleList } from "@/hooks/use-article-list";
import {
  buildMarkAllItemsReadPayload,
  useMarkAllItemsRead,
  useMarkItemsRead,
  useMarkItemsUnread,
} from "@/queries/items";
import { queryKeys } from "@/queries/keys";
import { useFeedLookup } from "@/queries/feeds";
import { useGroups } from "@/queries/groups";
import { useCreateBookmark, useDeleteBookmark } from "@/queries/bookmarks";
import { getFaviconUrl } from "@/lib/api/favicon";
import { useI18n } from "@/lib/i18n";
import { shouldAutoFetchEmptyPage } from "@/lib/query-status";
import {
  getScopeUnreadCount,
  resolveNextUnreadTarget,
  type UnreadNavigationTarget,
} from "@/lib/unread-navigation";
import { QueryErrorBanner, QueryErrorState } from "@/components/query-error";
import {
  articleListKey,
  useArticlePinsStore,
  useReadStatePins,
} from "@/store";
import type { Feed, Item } from "@/lib/api";

const MARK_READ_UNDO_DURATION_MS = 10_000;

type MarkReadOrigin = {
  feedId: number | null;
  groupId: number | null;
  filter: ArticleFilter;
};

export function ArticleList() {
  const { t } = useI18n();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const {
    articleFilter,
    setArticleFilter,
    selectedFeedId,
    selectedGroupId,
    selectedArticleId,
    setSelectedArticle,
    navigateToList,
  } = useUrlState();

  const {
    articles,
    hasMore,
    isLoading,
    isLoadingMore,
    isError,
    isFetchNextPageError,
    retry,
    fetchNextPage,
    getBookmarkByItemId,
  } = useArticleList({
    feedId: selectedFeedId,
    groupId: selectedGroupId,
    articleFilter,
  });

  const { data: groups = [] } = useGroups();
  const {
    feeds,
    getFeedById,
    isLoading: isFeedsLoading,
    isSuccess: isFeedsSuccess,
    isError: isFeedsError,
    refetch: refetchFeeds,
  } = useFeedLookup();
  const markItemsRead = useMarkItemsRead();
  const markItemsUnread = useMarkItemsUnread();
  const markAllItemsRead = useMarkAllItemsRead();
  const createBookmark = useCreateBookmark();
  const deleteBookmark = useDeleteBookmark();
  const [markAllConfirmOpen, setMarkAllConfirmOpen] = useState(false);

  const { pinRead, unpinRead } = useReadStatePins({
    feedId: selectedFeedId,
    groupId: selectedGroupId,
    articleFilter,
  });

  const pinsKey = articleListKey(selectedFeedId, selectedGroupId, articleFilter);
  const clearPins = useArticlePinsStore((s) => s.clearPins);

  // Leaving this exact list (filter/feed/group change or unmount) commits the
  // read markings: pinned rows stop being shown in the unread view. Only the
  // list itself clears pins — the drawer stays mounted on other pages and must
  // not commit on its own lifecycle.
  useEffect(() => {
    return () => {
      clearPins(pinsKey);
    };
  }, [pinsKey, clearPins]);

  const articleIds = articles.map((a) => a.id);
  useArticleNavigation(articleIds, {
    enabled: selectedArticleId === null,
  });

  // When every loaded row has been read away (hidden from the unread view),
  // keep pulling deeper pages instead of dead-ending on an empty list while
  // the server still has unread items.
  // Guarded on non-error states so failed requests do not spin an infinite loop.
  useEffect(() => {
    if (
      shouldAutoFetchEmptyPage({
        isLoading,
        isLoadingMore,
        isError,
        isFetchNextPageError,
        itemCount: articles.length,
        hasMore,
      })
    ) {
      fetchNextPage();
    }
  }, [
    articles.length,
    fetchNextPage,
    hasMore,
    isError,
    isFetchNextPageError,
    isLoading,
    isLoadingMore,
  ]);

  let title = t("article.list.all");
  let scopeUnreadCount = feeds.reduce((sum, f) => sum + f.unread_count, 0);
  if (selectedFeedId !== null) {
    const feed = getFeedById(selectedFeedId);
    title = feed?.name ?? t("article.feedFallback");
    scopeUnreadCount = feed?.unread_count ?? 0;
  } else if (selectedGroupId !== null) {
    const group = groups.find((g) => g.id === selectedGroupId);
    title = group?.name ?? t("article.groupFallback");
    scopeUnreadCount = feeds.reduce(
      (sum, f) => (f.group_id === selectedGroupId ? sum + f.unread_count : sum),
      0,
    );
  }

  const unreadCount = articles.filter((a) => a.unread).length;
  const isMarkingRead = markItemsRead.isPending || markAllItemsRead.isPending;
  const canMarkAllRead = scopeUnreadCount > 0 || unreadCount > 0;
  const hasArticleError = isError || isFetchNextPageError;
  const hasNoFeeds = !isFeedsLoading && !isFeedsError && feeds.length === 0;

  const handleToggleRead = useCallback(
    async (article: Item) => {
      if (article.id <= 0) return;

      try {
        if (article.unread) {
          // Pin before the request so the optimistic unread flip cannot
          // drop the row (still visible for undo) before the pin lands.
          pinRead([article.id]);
          await markItemsRead.mutateAsync([article.id]);
        } else {
          await markItemsUnread.mutateAsync([article.id]);
          unpinRead([article.id]);
        }
      } catch (error) {
        console.error("Failed to toggle read status:", error);
      }
    },
    [markItemsRead, markItemsUnread, pinRead, unpinRead],
  );

  const handleToggleStar = useCallback(
    async (article: Item) => {
      const bookmarkId =
        article.bookmark_id ?? getBookmarkByItemId(article.id)?.id ?? null;

      if (bookmarkId !== null) {
        try {
          await deleteBookmark.mutateAsync({
            bookmarkId,
            itemId: article.id,
          });
        } catch {
          toast.error(t("article.toast.unstarFailed"));
        }
        return;
      }

      try {
        await createBookmark.mutateAsync(article);
      } catch {
        toast.error(t("article.toast.starFailed"));
      }
    },
    [createBookmark, deleteBookmark, getBookmarkByItemId, t],
  );

  const applyTargetNavigation = (target: UnreadNavigationTarget) => {
    if (target.kind === "feed") {
      navigateToList({
        feedId: target.feedId,
        groupId: null,
        filter: "unread",
        articleId: null,
      });
    } else if (target.kind === "group") {
      navigateToList({
        groupId: target.groupId,
        feedId: null,
        filter: "unread",
        articleId: null,
      });
    } else {
      navigateToList({
        feedId: null,
        groupId: null,
        filter: "all",
        articleId: null,
      });
    }
  };

  const showMarkReadUndoToast = ({
    message,
    ids,
    origin,
    didAdvance,
    restorePinsKey,
  }: {
    message: string;
    ids: number[];
    origin: MarkReadOrigin;
    didAdvance: boolean;
    restorePinsKey?: string;
  }) => {
    if (ids.length === 0) return;
    let undoPending = false;
    toast.success(message, {
      duration: MARK_READ_UNDO_DURATION_MS,
      action: {
        label: t("article.toast.undo"),
        onClick: () => {
          if (undoPending) return;
          undoPending = true;
          void (async () => {
            try {
              await markItemsUnread.mutateAsync(ids);
              if (restorePinsKey) {
                useArticlePinsStore
                  .getState()
                  .unpinItems(restorePinsKey, ids);
              } else {
                await Promise.all([
                  qc.invalidateQueries({ queryKey: queryKeys.items.all }),
                  qc.invalidateQueries({ queryKey: queryKeys.bookmarks.lists() }),
                ]);
              }
              if (didAdvance) {
                navigateToList({
                  feedId: origin.feedId,
                  groupId: origin.groupId,
                  filter: origin.filter,
                  articleId: null,
                });
              }
            } catch (error) {
              undoPending = false;
              console.error("Failed to undo mark as read:", error);
              toast.error(t("article.toast.undoFailed"));
            }
          })();
        },
      },
    });
  };

  const handleMarkLoadedAsRead = async () => {
    const unreadIds = articles
      .filter((a) => a.unread && a.id > 0)
      .map((a) => a.id);

    if (unreadIds.length === 0) return;

    const origin: MarkReadOrigin = {
      feedId: selectedFeedId,
      groupId: selectedGroupId,
      filter: articleFilter,
    };
    const beforeFeeds = feeds;

    // Pin before the request so optimistic updates do not temporarily empty
    // the list and trigger shouldAutoFetchEmptyPage before pins land.
    pinRead(unreadIds);
    try {
      await markItemsRead.mutateAsync(unreadIds);

      const afterFeeds =
        qc.getQueryData<Feed[]>(queryKeys.feeds.list()) ?? beforeFeeds;
      const remainingScopeUnread = getScopeUnreadCount(
        afterFeeds,
        selectedFeedId,
        selectedGroupId,
      );
      const nextTarget =
        articleFilter === "unread" &&
        (selectedFeedId !== null || selectedGroupId !== null) &&
        remainingScopeUnread === 0
          ? resolveNextUnreadTarget({
              groups,
              beforeFeeds,
              afterFeeds,
              isFeedsSuccess,
              isFeedsError,
              selectedFeedId,
              selectedGroupId,
              articleFilter,
            })
          : null;

      const didAdvance = nextTarget !== null;
      if (didAdvance) {
        clearPins(pinsKey);
        applyTargetNavigation(nextTarget);
      }

      showMarkReadUndoToast({
        message: t("article.toast.markLoadedReadUndo", {
          count: unreadIds.length,
        }),
        ids: unreadIds,
        origin,
        didAdvance,
        restorePinsKey: didAdvance ? undefined : pinsKey,
      });
    } catch (error) {
      unpinRead(unreadIds);
      console.error("Failed to mark loaded items as read:", error);
      toast.error(t("article.toast.markLoadedReadFailed"));
    }
  };

  const executeMarkAllAsRead = async () => {
    const origin: MarkReadOrigin = {
      feedId: selectedFeedId,
      groupId: selectedGroupId,
      filter: articleFilter,
    };
    const scopeTitle = title;
    const beforeFeeds = feeds;
    const isScopedUnread =
      articleFilter === "unread" &&
      (selectedFeedId !== null || selectedGroupId !== null);
    const currentUnreadIds = isScopedUnread
      ? articles.filter((a) => a.unread && a.id > 0).map((a) => a.id)
      : [];

    if (isScopedUnread && currentUnreadIds.length > 0) {
      // Keep rows pinned while the request settles so scoped feeds/groups do
      // not briefly flash an empty state before auto-advancing.
      pinRead(currentUnreadIds);
    }

    try {
      const affectedIds = await markAllItemsRead.mutateAsync(
        buildMarkAllItemsReadPayload(selectedFeedId, selectedGroupId),
      );
      setMarkAllConfirmOpen(false);

      const afterFeeds =
        qc.getQueryData<Feed[]>(queryKeys.feeds.list()) ?? beforeFeeds;
      const nextTarget = resolveNextUnreadTarget({
        groups,
        beforeFeeds,
        afterFeeds,
        isFeedsSuccess,
        isFeedsError,
        selectedFeedId,
        selectedGroupId,
        articleFilter,
      });

      if (nextTarget) {
        applyTargetNavigation(nextTarget);
      } else {
        clearPins(pinsKey);
      }

      showMarkReadUndoToast({
        message: t("article.toast.markAllReadUndo", {
          scope: scopeTitle,
          count: affectedIds.length,
        }),
        ids: affectedIds,
        origin,
        didAdvance: nextTarget !== null,
      });
    } catch (error) {
      if (isScopedUnread && currentUnreadIds.length > 0) {
        unpinRead(currentUnreadIds);
      }
      console.error("Failed to mark all as read:", error);
      toast.error(t("article.toast.markAllReadFailed"));
    }
  };

  const handleMarkAllButtonClick = () => {
    if (selectedFeedId === null && selectedGroupId === null) {
      setMarkAllConfirmOpen(true);
      return;
    }
    void executeMarkAllAsRead();
  };

  return (
    <div className="flex h-full flex-col">
      <ContentHeader>
        <div className="flex min-w-0 items-center gap-1">
          <SidebarTrigger />
          <h2 className="truncate text-lg font-semibold">{title}</h2>
        </div>
        <div className="flex shrink-0 items-center">
          <Button
            variant="outline"
            size="sm"
            onClick={handleMarkAllButtonClick}
            disabled={!canMarkAllRead || isMarkingRead}
            className="gap-1.5 rounded-r-none text-xs focus-visible:z-10"
          >
            <CheckCheck className="h-4 w-4" />
            {t("article.list.markAllRead")}
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger
              disabled={unreadCount === 0 || isMarkingRead}
              render={
                <Button
                  variant="outline"
                  size="sm"
                  aria-label={t("article.list.moreReadActions")}
                  className="-ml-px rounded-l-none px-1.5 focus-visible:z-10"
                >
                  <ChevronDown className="h-3.5 w-3.5" />
                </Button>
              }
            />
            <DropdownMenuContent align="end" className="w-auto">
              <DropdownMenuItem
                disabled={unreadCount === 0 || isMarkingRead}
                onClick={handleMarkLoadedAsRead}
              >
                <CheckCheck className="h-4 w-4" />
                {t("article.list.markLoadedRead")}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </ContentHeader>

      {/* Article area with filter tabs */}
      <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-hidden px-4 py-4 sm:px-6">
        {/* Filter tabs - hidden when no articles exist */}
        {!hasNoFeeds && (articles.length > 0 || articleFilter !== "all") && (
          <Tabs
            value={articleFilter}
            onValueChange={(v) => setArticleFilter(v as ArticleFilter)}
          >
            <TabsList>
              <TabsTrigger value="all">{t("article.filter.all")}</TabsTrigger>
              <TabsTrigger value="unread">{t("article.filter.unread")}</TabsTrigger>
              <TabsTrigger value="starred">
                {t("article.filter.starred")}
              </TabsTrigger>
            </TabsList>
          </Tabs>
        )}

            {/* Article list */}
        <ScrollArea className="min-h-0 flex-1">
          <div>
            {isLoading || (articles.length === 0 && hasMore && !hasArticleError) ? (
              <div className="space-y-2 p-2">
                {[1, 2, 3, 4, 5].map((i) => (
                  <div
                    key={i}
                    className="h-24 animate-pulse rounded-md bg-accent"
                  />
                ))}
              </div>
            ) : articles.length === 0 ? (
              hasArticleError ? (
                <QueryErrorState
                  message={t("article.list.loadFailed")}
                  onRetry={retry}
                />
              ) : isFeedsError && feeds.length === 0 ? (
                <QueryErrorState
                  message={t("sidebar.loadFailed")}
                  onRetry={() => void refetchFeeds()}
                />
              ) : hasNoFeeds ? (
                <div className="flex flex-col items-center justify-center gap-3 py-12 text-center">
                  <p className="text-sm text-muted-foreground">
                    {t("article.list.noFeeds")}
                  </p>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => navigate({ to: "/feeds" })}
                  >
                    {t("article.list.openFeedManagement")}
                  </Button>
                </div>
              ) : (
                <div className="flex flex-col items-center justify-center py-12 text-center">
                  <p className="text-sm text-muted-foreground">
                    {t("article.list.noArticles")}
                  </p>
                </div>
              )
            ) : (
              <>
                {isError && !isFetchNextPageError && (
                  <div className="p-2">
                    <QueryErrorBanner
                      message={t("article.list.loadFailed")}
                      onRetry={retry}
                    />
                  </div>
                )}
                {articles.map((article) => {
                  const feed = getFeedById(article.feed_id);
                  const bookmark = getBookmarkByItemId(article.id);

                  return (
                    <ArticleItem
                      key={article.id}
                      article={article}
                      selectedArticleId={selectedArticleId}
                      onSelectArticle={setSelectedArticle}
                      onToggleRead={handleToggleRead}
                      onToggleStar={handleToggleStar}
                      canToggleRead={article.id > 0}
                      isStarred={
                        article.bookmark_id !== null || bookmark !== undefined
                      }
                      feedName={feed?.name ?? bookmark?.feed_name ?? t("common.unknown")}
                      feedFaviconUrl={
                        feed ? getFaviconUrl(feed.link, feed.site_url) : null
                      }
                    />
                  );
                })}
                {hasMore && isFetchNextPageError ? (
                  <div className="p-2">
                    <QueryErrorBanner
                      message={t("article.list.loadFailed")}
                      onRetry={retry}
                    />
                  </div>
                ) : (
                  hasMore && (
                    <div className="flex justify-center py-4">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => fetchNextPage()}
                        disabled={isLoadingMore}
                        className="gap-2"
                      >
                        {isLoadingMore && (
                          <Loader2 className="h-4 w-4 animate-spin" />
                        )}
                        {isLoadingMore
                          ? t("article.list.loading")
                          : t("article.list.loadMore")}
                      </Button>
                    </div>
                  )
                )}
              </>
            )}
          </div>
        </ScrollArea>
      </div>

      <Dialog
        open={markAllConfirmOpen}
        onOpenChange={(open) => {
          if (!markAllItemsRead.isPending) {
            setMarkAllConfirmOpen(open);
          }
        }}
      >
        <DialogContent
          data-hotkey-ignore="true"
          className="sm:max-w-[425px]"
        >
          <DialogHeader>
            <DialogTitle>{t("article.list.markAllDialog.title")}</DialogTitle>
            <DialogDescription>
              {t("article.list.markAllDialog.description", { scope: title })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              disabled={markAllItemsRead.isPending}
              onClick={() => setMarkAllConfirmOpen(false)}
            >
              {t("common.cancel")}
            </Button>
            <Button
              disabled={markAllItemsRead.isPending}
              onClick={executeMarkAllAsRead}
            >
              {markAllItemsRead.isPending
                ? t("article.list.markAllDialog.confirming")
                : t("article.list.markAllRead")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
