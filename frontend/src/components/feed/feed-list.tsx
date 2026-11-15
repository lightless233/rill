import {
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
} from "react";
import { useLocation } from "@tanstack/react-router";
import {
  ChevronsDownUp,
  ChevronsUpDown,
  Inbox,
  Layers,
  Star,
} from "lucide-react";
import { toast } from "sonner";
import { QueryErrorBanner } from "@/components/query-error";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { isArticleFilter } from "@/lib/article-filter";
import { sortFeedsUnreadFirst } from "@/lib/feed-sort";
import { shouldHideDefaultGroup } from "@/lib/group-visibility";
import { useGroups, useReorderGroups } from "@/queries/groups";
import { useAppInfo } from "@/queries/app";
import { useFeedLookup, useUnreadCounts } from "@/queries/feeds";
import { useUrlState } from "@/hooks/use-url-state";
import { useI18n } from "@/lib/i18n";
import { useUIStore } from "@/store";
import { cn } from "@/lib/utils";
import { FeedGroup } from "./feed-group";
import { FeedItem } from "./feed-item";
import { useGroupReorder } from "./use-group-reorder";

const hasNativeFocusGroup =
  typeof HTMLElement !== "undefined" && "focusGroup" in HTMLElement.prototype;

export function FeedList() {
  const { t } = useI18n();
  const groupListId = useId();
  const contentRootRef = useRef<HTMLDivElement | null>(null);
  const expandButtonRef = useRef<HTMLButtonElement | null>(null);
  const collapseButtonRef = useRef<HTMLButtonElement | null>(null);
  const autoExpandedFeedIdRef = useRef<number | null>(null);
  const lastScrolledSelectionKeyRef = useRef<string | null>(null);
  const [activeToolbarIndex, setActiveToolbarIndex] = useState(0);

  const collapsedGroupIds = useUIStore((s) => s.collapsedGroupIds);
  const setCollapsedGroupIds = useUIStore((s) => s.setCollapsedGroupIds);
  const setGroupOpen = useUIStore((s) => s.setGroupOpen);
  const collapsedSet = useMemo(
    () => new Set(collapsedGroupIds),
    [collapsedGroupIds],
  );

  const {
    data: groups = [],
    isLoading: isGroupsLoading,
    isError: isGroupsError,
    refetch: refetchGroups,
  } = useGroups();
  const reorderGroups = useReorderGroups();
  const {
    feeds,
    getFeedById,
    getFeedsByGroup,
    isLoading: isFeedsLoading,
    isSuccess: isFeedsSuccess,
    isError: isFeedsError,
    refetch: refetchFeeds,
  } = useFeedLookup();
  const { getTotalUnreadCount } = useUnreadCounts();
  const { data: appInfo } = useAppInfo();
  const starredTotal = appInfo?.bookmark_count;
  const isLoading = isGroupsLoading || isFeedsLoading;
  const hasLoadError = isGroupsError || isFeedsError;

  const handleRetryLoad = () => {
    if (isGroupsError) void refetchGroups();
    if (isFeedsError) void refetchFeeds();
  };
  const {
    selectedFeedId,
    selectedGroupId,
    articleFilter,
    selectTopLevelFilter,
  } = useUrlState();
  const { pathname } = useLocation();
  const firstPathSegment = pathname.split("/").filter(Boolean)[0];
  const isOnHomePage =
    typeof firstPathSegment === "string" && isArticleFilter(firstPathSegment);
  const isTopLevelSelected =
    isOnHomePage && selectedFeedId === null && selectedGroupId === null;
  const totalUnread = getTotalUnreadCount();
  const starredCount = starredTotal;

  const visibleGroups = useMemo(() => {
    return groups.filter((group) => {
      const groupFeeds = getFeedsByGroup(group.id);
      return !shouldHideDefaultGroup({
        groupId: group.id,
        unfilteredGroupFeedCount: groupFeeds.length,
        isFeedsSuccess,
        isFeedsError,
      });
    });
  }, [groups, getFeedsByGroup, isFeedsSuccess, isFeedsError]);

  const sortedFeedsByGroup = useMemo(() => {
    const map = new Map<number, typeof feeds>();
    for (const group of visibleGroups) {
      map.set(group.id, sortFeedsUnreadFirst(getFeedsByGroup(group.id)));
    }
    return map;
  }, [visibleGroups, getFeedsByGroup]);

  const sortedUngroupedFeeds = useMemo(
    () => sortFeedsUnreadFirst(feeds.filter((f) => f.group_id === 0)),
    [feeds],
  );

  const allExpanded =
    visibleGroups.length === 0 ||
    visibleGroups.every((g) => !collapsedSet.has(g.id));
  const allCollapsed =
    visibleGroups.length === 0 ||
    visibleGroups.every((g) => collapsedSet.has(g.id));

  const handleExpandAll = useCallback(() => {
    if (allExpanded) return;
    setCollapsedGroupIds([]);
  }, [allExpanded, setCollapsedGroupIds]);

  const handleCollapseAll = useCallback(() => {
    if (allCollapsed) return;
    setCollapsedGroupIds(visibleGroups.map((g) => g.id));
  }, [allCollapsed, setCollapsedGroupIds, visibleGroups]);

  // Expand the parent group once when a feed selection resolves inside a
  // collapsed group without fighting subsequent manual collapses on the same feed.
  useEffect(() => {
    if (selectedFeedId === null) {
      autoExpandedFeedIdRef.current = null;
      return;
    }
    const selectedFeed = getFeedById(selectedFeedId);
    if (!selectedFeed) return;
    if (autoExpandedFeedIdRef.current === selectedFeedId) return;
    autoExpandedFeedIdRef.current = selectedFeedId;
    if (selectedFeed.group_id > 0) {
      setGroupOpen(selectedFeed.group_id, true);
    }
  }, [selectedFeedId, getFeedById, setGroupOpen]);

  const activeSelectionKey =
    selectedFeedId !== null
      ? `feed:${selectedFeedId}`
      : selectedGroupId !== null
        ? `group:${selectedGroupId}`
        : isTopLevelSelected
          ? `top:${articleFilter}`
          : null;

  const activeTargetSelector =
    selectedFeedId !== null
      ? `[data-feed-item="${selectedFeedId}"]`
      : selectedGroupId !== null
        ? `[data-group-row="${selectedGroupId}"]`
        : isTopLevelSelected
          ? `[data-top-filter="${articleFilter}"]`
          : null;

  // Scroll the sidebar viewport only when the active target is outside it.
  useEffect(() => {
    if (!activeSelectionKey || !activeTargetSelector) {
      lastScrolledSelectionKeyRef.current = null;
      return;
    }
    if (lastScrolledSelectionKeyRef.current === activeSelectionKey) {
      return;
    }
    const root = contentRootRef.current;
    if (!root) return;
    const target = root.querySelector<HTMLElement>(activeTargetSelector);
    if (!target) return;

    lastScrolledSelectionKeyRef.current = activeSelectionKey;

    const viewport = root.closest<HTMLElement>(
      '[data-slot="scroll-area-viewport"]',
    );
    if (!viewport) return;

    const viewportRect = viewport.getBoundingClientRect();
    const targetRect = target.getBoundingClientRect();
    if (viewportRect.height <= 0 || targetRect.height <= 0) {
      return;
    }

    const padding = 8;
    const targetTop = viewport.scrollTop + (targetRect.top - viewportRect.top);
    const targetBottom = targetTop + targetRect.height;

    let nextTop: number | null = null;
    if (targetTop < viewport.scrollTop) {
      nextTop = Math.max(0, targetTop - padding);
    } else if (targetBottom > viewport.scrollTop + viewport.clientHeight) {
      nextTop = Math.max(0, targetBottom - viewport.clientHeight + padding);
    }

    if (nextTop !== null) {
      if (typeof viewport.scrollTo === "function") {
        viewport.scrollTo({ top: nextTop, behavior: "smooth" });
      } else {
        viewport.scrollTop = nextTop;
      }
    }
  }, [
    activeSelectionKey,
    activeTargetSelector,
    collapsedSet,
    feeds.length,
    visibleGroups.length,
  ]);

  const handleToolbarKeyDown = useCallback(
    (event: ReactKeyboardEvent<HTMLDivElement>) => {
      if (hasNativeFocusGroup) return;
      const buttons = [expandButtonRef.current, collapseButtonRef.current];
      const currentIdx = buttons.findIndex(
        (btn) => btn === document.activeElement,
      );
      if (currentIdx === -1) return;

      const isRtl =
        window.getComputedStyle(event.currentTarget).direction === "rtl";
      const nextStep =
        event.key === "ArrowRight"
          ? isRtl
            ? -1
            : 1
          : event.key === "ArrowLeft"
            ? isRtl
              ? 1
              : -1
            : null;

      let targetIdx: number | null = null;
      if (nextStep !== null) {
        targetIdx = (currentIdx + nextStep + buttons.length) % buttons.length;
      } else if (event.key === "Home") {
        targetIdx = 0;
      } else if (event.key === "End") {
        targetIdx = buttons.length - 1;
      }

      if (targetIdx !== null) {
        event.preventDefault();
        setActiveToolbarIndex(targetIdx);
        buttons[targetIdx]?.focus();
      }
    },
    [],
  );

  const handleCommitReorder = useCallback(
    (orderedGroups: typeof groups) => {
      reorderGroups.mutate(orderedGroups, {
        onError: () => {
          toast.error(t("group.toast.reorderFailed"));
        },
      });
    },
    [reorderGroups, t],
  );

  const {
    listRef,
    renderedVisibleGroups,
    draggingGroupId,
    grabbedGroupId,
    statusMessage,
    onHandlePointerDown,
    onHandleClick,
    onHandleKeyDown,
    onHandleBlur,
  } = useGroupReorder({
    groups,
    visibleGroups,
    onCommitReorder: handleCommitReorder,
    t,
  });

  const topFilters: Array<{
    value: "all" | "unread" | "starred";
    label: string;
    count?: number;
    icon: typeof Inbox;
  }> = [
    {
      value: "unread",
      label: t("article.filter.unread"),
      count: totalUnread,
      icon: Inbox,
    },
    {
      value: "starred",
      label: t("article.filter.starred"),
      count: starredCount,
      icon: Star,
    },
    {
      value: "all",
      label: t("article.filter.all"),
      icon: Layers,
    },
  ];

  if (isLoading && groups.length === 0 && !hasLoadError) {
    return (
      <div className="flex-1 p-4">
        <div className="space-y-2">
          {[1, 2, 3].map((i) => (
            <div key={i} className="h-8 animate-pulse rounded-md bg-accent" />
          ))}
        </div>
      </div>
    );
  }

  return (
    <ScrollArea className="min-h-0 flex-1 w-full min-w-0 overflow-hidden">
      <div ref={contentRootRef} className="w-full min-w-0 p-2 space-y-0.5">
        {/* Top-level filters */}
        <div className="space-y-0.5">
          {topFilters.map(({ value, label, count, icon: Icon }) => (
            <button
              key={value}
              data-top-filter={value}
              onClick={() => selectTopLevelFilter(value)}
              className={cn(
                "flex w-full min-w-0 items-center gap-1.5 rounded-md px-2 py-1 text-left text-sm transition-colors",
                isTopLevelSelected && articleFilter === value
                  ? "bg-accent text-accent-foreground"
                  : "hover:bg-accent/50",
              )}
            >
              <Icon className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
              <span className="min-w-0 flex-1">{label}</span>
              {typeof count === "number" && (
                <span className="shrink-0 text-[11px] text-muted-foreground">
                  {count}
                </span>
              )}
            </button>
          ))}
        </div>

        {/* Feeds header */}
        <div className="mt-2 flex items-center justify-between px-2 py-1">
          <span className="text-[11px] font-medium text-muted-foreground">
            {t("search.group.feeds")}
          </span>
          {visibleGroups.length > 0 && (
            <TooltipProvider delay={300}>
              <div
                role="toolbar"
                aria-label={t("sidebar.feedsActions")}
                aria-controls={groupListId}
                onKeyDown={handleToolbarKeyDown}
                {...({ focusgroup: "toolbar wrap" } as Record<string, string>)}
                className="flex items-center gap-0.5"
              >
                <Tooltip>
                  <TooltipTrigger
                    render={
                      <button
                        ref={expandButtonRef}
                        type="button"
                        aria-label={t("sidebar.expandAll")}
                        aria-disabled={allExpanded}
                        tabIndex={
                          hasNativeFocusGroup
                            ? undefined
                            : activeToolbarIndex === 0
                              ? 0
                              : -1
                        }
                        onFocus={() => setActiveToolbarIndex(0)}
                        onClick={handleExpandAll}
                        className="inline-flex min-h-6 min-w-6 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 aria-disabled:cursor-default aria-disabled:opacity-40 aria-disabled:hover:bg-transparent aria-disabled:hover:text-muted-foreground"
                      />
                    }
                  >
                    <ChevronsUpDown className="h-3.5 w-3.5" aria-hidden="true" />
                  </TooltipTrigger>
                  <TooltipContent side="bottom">
                    {t("sidebar.expandAll")}
                  </TooltipContent>
                </Tooltip>
                <Tooltip>
                  <TooltipTrigger
                    render={
                      <button
                        ref={collapseButtonRef}
                        type="button"
                        aria-label={t("sidebar.collapseAll")}
                        aria-disabled={allCollapsed}
                        tabIndex={
                          hasNativeFocusGroup
                            ? undefined
                            : activeToolbarIndex === 1
                              ? 0
                              : -1
                        }
                        onFocus={() => setActiveToolbarIndex(1)}
                        onClick={handleCollapseAll}
                        className="inline-flex min-h-6 min-w-6 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 aria-disabled:cursor-default aria-disabled:opacity-40 aria-disabled:hover:bg-transparent aria-disabled:hover:text-muted-foreground"
                      />
                    }
                  >
                    <ChevronsDownUp className="h-3.5 w-3.5" aria-hidden="true" />
                  </TooltipTrigger>
                  <TooltipContent side="bottom">
                    {t("sidebar.collapseAll")}
                  </TooltipContent>
                </Tooltip>
              </div>
            </TooltipProvider>
          )}
        </div>

        <p role="status" className="sr-only">
          {statusMessage}
        </p>

        {hasLoadError && (
          <QueryErrorBanner
            message={t("sidebar.loadFailed")}
            onRetry={handleRetryLoad}
            className="mx-1 mb-1"
          />
        )}

        {/* Feed groups */}
        <div
          id={groupListId}
          ref={listRef}
          className="group-reorder-list relative w-full min-w-0 space-y-0.5"
        >
          {renderedVisibleGroups.map((group) => {
            const groupFeeds = sortedFeedsByGroup.get(group.id) ?? [];

            return (
              <FeedGroup
                key={group.id}
                groupId={group.id}
                name={group.name}
                feeds={groupFeeds}
                isOpen={!collapsedSet.has(group.id)}
                onOpenChange={(open) => setGroupOpen(group.id, open)}
                isReorderable={visibleGroups.length > 1}
                isDragging={draggingGroupId === group.id}
                isGrabbed={grabbedGroupId === group.id}
                reorderHandleLabel={t("group.reorder.handleLabel", {
                  name: group.name,
                })}
                onReorderPointerDown={(e) =>
                  onHandlePointerDown(group.id, group.name, e)
                }
                onReorderClick={(e) => onHandleClick(group.id, group.name, e)}
                onReorderKeyDown={(e) =>
                  onHandleKeyDown(group.id, group.name, e)
                }
                onReorderBlur={() => onHandleBlur(group.id, group.name)}
              />
            );
          })}

          {/* Ungrouped feeds (group_id = 0) */}
          {sortedUngroupedFeeds.map((feed) => (
            <FeedItem key={feed.id} feed={feed} />
          ))}
        </div>
      </div>
    </ScrollArea>
  );
}
