import { useState } from "react";
import {
  Circle,
  CircleCheck,
  ChevronLeft,
  ChevronRight,
  ChevronUp,
  ExternalLink,
  Languages,
  Loader2,
  RotateCw,
  Sparkles,
  Star,
  X,
} from "lucide-react";
import { toast } from "sonner";
import { QueryErrorBanner, QueryErrorState } from "@/components/query-error";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { Button, buttonVariants } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useUrlState } from "@/hooks/use-url-state";
import type { Item } from "@/lib/api";
import {
  useItem,
  useMarkItemsRead,
  useMarkItemsUnread,
} from "@/queries/items";
import { useFeedLookup } from "@/queries/feeds";
import {
  useCreateBookmark,
  useDeleteBookmark,
} from "@/queries/bookmarks";
import { useArticleList } from "@/hooks/use-article-list";
import { useArticleNavigation } from "@/hooks/use-keyboard";
import { useAutoMarkRead } from "@/hooks/use-auto-mark-read";
import { useArticleSummary } from "@/hooks/use-article-summary";
import { useArticleTranslation } from "@/hooks/use-article-translation";
import { useI18n } from "@/lib/i18n";
import { cn, formatDate } from "@/lib/utils";
import { getFaviconUrl } from "@/lib/api/favicon";
import { FeedFavicon } from "@/components/feed/feed-favicon";
import { toSafeExternalUrl } from "@/lib/safe-url";
import { useReadStatePins, usePreferencesStore } from "@/store";
import { autoMarkReadDelayMs } from "@/store/preferences";
import type { ArticleViewMode } from "@/lib/translation";

export function ArticleDrawer() {
  const { t } = useI18n();
  const {
    selectedArticleId,
    setSelectedArticle,
    setSelectedFeed,
    selectedFeedId,
    selectedGroupId,
    articleFilter,
  } = useUrlState();
  const { getFeedById } = useFeedLookup();

  const { articles, isStarredMode, getBookmarkByItemId } =
    useArticleList({
      feedId: selectedFeedId,
      groupId: selectedGroupId,
      articleFilter,
    });

  const markRead = useMarkItemsRead();
  const markUnread = useMarkItemsUnread();
  const createBookmark = useCreateBookmark();
  const deleteBookmark = useDeleteBookmark();

  const { pinRead, unpinRead } = useReadStatePins({
    feedId: selectedFeedId,
    groupId: selectedGroupId,
    articleFilter,
  });

  const articleIds = articles.map((a) => a.id);

  const storeArticle = selectedArticleId
    ? (articles.find((i) => i.id === selectedArticleId) ?? null)
    : null;

  const shouldFetchArticle =
    selectedArticleId !== null &&
    selectedArticleId > 0 &&
    (isStarredMode || storeArticle === null);
  const {
    data: fetchedArticle,
    isLoading: isItemLoading,
    isError: isItemError,
    refetch: refetchItem,
  } = useItem(selectedArticleId, shouldFetchArticle);

  const article: Item | null =
    (isStarredMode ? fetchedArticle ?? storeArticle : storeArticle ?? fetchedArticle) ??
    null;
  // Read state comes from the in-hand row (list/bookmark cache), not the
  // detail refetch: in starred mode the fetched detail can disagree with the
  // row, and the header toggle, the auto-mark timer, and the veto must all
  // watch the same unread bit or a manual mark-unread gets overwritten.
  const readArticle = storeArticle ?? fetchedArticle ?? null;
  const canToggleRead = readArticle !== null && readArticle.id > 0;
  // Preserve the closing article snapshot during Sheet's exit transition so the
  // drawer slides out with its content intact instead of flashing blank.
  const [closingSnapshot, setClosingSnapshot] = useState<{
    article: Item;
    unread: boolean;
  } | null>(null);
  const activeUnread = readArticle?.unread ?? false;

  if (article !== null) {
    if (
      closingSnapshot?.article !== article ||
      closingSnapshot.unread !== activeUnread
    ) {
      setClosingSnapshot({ article, unread: activeUnread });
    }
  } else if (selectedArticleId !== null && closingSnapshot !== null) {
    setClosingSnapshot(null);
  }

  const displayedArticle =
    article ??
    (selectedArticleId === null ? (closingSnapshot?.article ?? null) : null);
  const displayedUnread =
    readArticle?.unread ??
    (selectedArticleId === null ? (closingSnapshot?.unread ?? false) : false);
  const displayedCanToggleRead =
    displayedArticle !== null && displayedArticle.id > 0;
  const feed = displayedArticle ? getFeedById(displayedArticle.feed_id) : null;
  const bookmark = displayedArticle
    ? getBookmarkByItemId(displayedArticle.id)
    : null;
  const activeBookmarkId = displayedArticle
    ? (displayedArticle.bookmark_id ?? bookmark?.id ?? null)
    : null;
  const starred = activeBookmarkId !== null;
  const safeArticleLink = displayedArticle
    ? toSafeExternalUrl(displayedArticle.link)
    : null;

  const {
    status: translationStatus,
    viewMode,
    setViewMode,
    translatedTitle,
    receivedSegments,
    totalSegments,
    renderedHtml,
    hasTranslatableContent,
    hasTranslation,
    startTranslation,
    cancelTranslation,
  } = useArticleTranslation(displayedArticle, safeArticleLink);

  const {
    status: summaryStatus,
    renderedSummaryHtml,
    summaryModel,
    hasSummary,
    hasSummarizableContent,
    isOpen: isSummaryOpen,
    setOpen: setSummaryOpen,
    startSummary,
    cancelSummary,
  } = useArticleSummary(displayedArticle);

  const autoMarkRead = usePreferencesStore((s) => s.autoMarkRead);
  const autoMarkDelayMs = autoMarkReadDelayMs(autoMarkRead);

  // The target is not gated on the setting: null means the drawer left the
  // article, which is exactly the event that must clear the veto. "Feature
  // off" travels as delayMs === null instead.
  useAutoMarkRead({
    target:
      readArticle && readArticle.id > 0
        ? { id: readArticle.id, unread: readArticle.unread }
        : null,
    delayMs: autoMarkDelayMs,
    // mutate and pinRead must share one synchronous turn (see hook docs).
    onMarkRead: (id) => {
      markRead.mutate([id]);
      pinRead([id]);
    },
    hasPendingMutation: () => markRead.isPending || markUnread.isPending,
  });

  const handleOpenChange = (open: boolean) => {
    if (!open) {
      setSelectedArticle(null);
    }
  };

  const handleToggleRead = async () => {
    if (!readArticle || !canToggleRead) return;
    try {
      if (readArticle.unread) {
        // Pin before the request: the optimistic unread flip drops the row
        // from the unread list (and with it this drawer's content) until
        // the pin lands — the await would flash a blank pane.
        pinRead([readArticle.id]);
        await markRead.mutateAsync([readArticle.id]);
      } else {
        await markUnread.mutateAsync([readArticle.id]);
        unpinRead([readArticle.id]);
      }
    } catch (error) {
      console.error("Failed to toggle read status:", error);
    }
  };

  const handleToggleStar = async () => {
    if (!article) return;
    if (activeBookmarkId !== null) {
      try {
        await deleteBookmark.mutateAsync({
          bookmarkId: activeBookmarkId,
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
  };

  const handleOpenOriginal = () => {
    if (!safeArticleLink) return;
    window.open(safeArticleLink, "_blank", "noopener,noreferrer");
  };

  const handleOpenFeed = () => {
    if (!article || article.feed_id <= 0) return;
    setSelectedFeed(article.feed_id);
  };

  const getLinkDomain = (url: string) => {
    try {
      return new URL(url).hostname;
    } catch {
      return url;
    }
  };

  const { goToNext, goToPrevious, hasNext, hasPrevious } =
    useArticleNavigation(articleIds, {
      enabled: selectedArticleId !== null,
      onToggleRead: () => {
        void handleToggleRead();
      },
      onToggleStar: () => {
        void handleToggleStar();
      },
      onOpenOriginal: handleOpenOriginal,
    });

  return (
    <Sheet
      open={selectedArticleId !== null}
      onOpenChange={handleOpenChange}
      onOpenChangeComplete={(open) => {
        if (!open) {
          setClosingSnapshot(null);
        }
      }}
    >
      <SheetContent
        side="right"
        className="data-[side=right]:w-full data-[side=right]:sm:max-w-[max(840px,60vw)] p-0"
        showCloseButton={false}
      >
        {displayedArticle ? (
          <div className="flex h-full flex-col">
            {/* Header */}
            <div className="flex items-center justify-between gap-2 border-b px-4 py-3 sm:px-6">
              <div className="flex flex-wrap items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={handleToggleRead}
                  disabled={!displayedCanToggleRead}
                  className="h-auto gap-1.5 px-2.5 py-1.5 text-[13px] font-medium text-muted-foreground"
                >
                  {displayedUnread ? (
                    <Circle className="h-4 w-4 text-muted-foreground" />
                  ) : (
                    <CircleCheck className="h-4 w-4 text-primary" />
                  )}
                  {displayedUnread
                    ? t("article.action.markRead")
                    : t("article.action.markUnread")}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={handleToggleStar}
                  className="h-auto gap-1.5 px-2.5 py-1.5 text-[13px] font-medium text-muted-foreground"
                >
                  <Star
                    className={`h-4 w-4 ${starred ? "fill-current text-amber-500" : ""}`}
                  />
                  {starred ? t("article.action.unstar") : t("article.action.star")}
                </Button>
                {safeArticleLink ? (
                  <a
                    href={safeArticleLink}
                    target="_blank"
                    rel="noopener noreferrer"
                    className={cn(
                      buttonVariants({ variant: "outline", size: "sm" }),
                      "h-auto gap-1.5 px-2.5 py-1.5 text-[13px] font-medium text-muted-foreground",
                    )}
                  >
                    <ExternalLink className="h-4 w-4" />
                    {t("article.action.original")}
                  </a>
                ) : (
                  <Button
                    variant="outline"
                    size="sm"
                    disabled
                    className="h-auto gap-1.5 px-2.5 py-1.5 text-[13px] font-medium text-muted-foreground"
                  >
                    <ExternalLink className="h-4 w-4" />
                    {t("article.action.original")}
                  </Button>
                )}

                {translationStatus === "translating" ? (
                  <div className="flex items-center gap-1.5">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={cancelTranslation}
                      className="h-auto gap-1.5 px-2.5 py-1.5 text-[13px] font-medium text-muted-foreground"
                    >
                      <Loader2 className="h-4 w-4 animate-spin" />
                      {totalSegments > 0
                        ? t("article.translation.translatingProgress", {
                            done: receivedSegments,
                            total: totalSegments,
                          })
                        : t("article.translation.translating")}
                    </Button>
                    {receivedSegments > 0 && (
                      <TranslationModeSwitcher
                        value={viewMode}
                        onChange={setViewMode}
                      />
                    )}
                  </div>
                ) : hasTranslation ? (
                  <div className="flex items-center gap-1.5">
                    <TranslationModeSwitcher
                      value={viewMode}
                      onChange={setViewMode}
                    />
                    <TooltipProvider delay={300}>
                      <Tooltip>
                        <TooltipTrigger
                          render={
                            <Button
                              variant="outline"
                              size="icon-sm"
                              onClick={() => void startTranslation({ force: true })}
                              aria-label={t("article.translation.retranslate")}
                              className="h-[30px] w-[30px] text-muted-foreground"
                            />
                          }
                        >
                          <RotateCw className="h-3.5 w-3.5" aria-hidden="true" />
                        </TooltipTrigger>
                        <TooltipContent side="bottom">
                          {t("article.translation.retranslate")}
                        </TooltipContent>
                      </Tooltip>
                    </TooltipProvider>
                  </div>
                ) : (
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={!hasTranslatableContent}
                    onClick={() => void startTranslation()}
                    className="h-auto gap-1.5 px-2.5 py-1.5 text-[13px] font-medium text-muted-foreground"
                  >
                    <Languages className="h-4 w-4" />
                    {t("article.action.translate")}
                  </Button>
                )}

                {summaryStatus === "summarizing" ? (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={cancelSummary}
                    className="h-auto gap-1.5 px-2.5 py-1.5 text-[13px] font-medium text-muted-foreground"
                  >
                    <Loader2 className="h-4 w-4 animate-spin" />
                    {t("article.summary.summarizing")}
                  </Button>
                ) : (
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={!hasSummarizableContent}
                    aria-pressed={isSummaryOpen}
                    onClick={() => {
                      if (hasSummary) {
                        setSummaryOpen(!isSummaryOpen);
                      } else {
                        void startSummary();
                      }
                    }}
                    className={cn(
                      "h-auto gap-1.5 px-2.5 py-1.5 text-[13px] font-medium text-muted-foreground",
                      hasSummary &&
                        isSummaryOpen &&
                        "border-primary/40 bg-primary/8 text-foreground",
                    )}
                  >
                    <Sparkles
                      className={cn("h-4 w-4", hasSummary && "text-primary")}
                    />
                    {t("article.action.summarize")}
                  </Button>
                )}
              </div>

              <SheetTitle className="sr-only">
                {displayedArticle.title}
              </SheetTitle>

              <Button
                variant="ghost"
                size="icon-sm"
                onClick={() => setSelectedArticle(null)}
                aria-label={t("common.cancel")}
              >
                <X className="h-[18px] w-[18px] text-muted-foreground" />
              </Button>
            </div>

            {/* Content */}
            <ScrollArea className="min-h-0 flex-1">
              <article className="min-w-0 px-5 py-6 sm:px-12 sm:py-8">
                {shouldFetchArticle && isItemError && (
                  <QueryErrorBanner
                    message={t("article.detail.loadFailed")}
                    onRetry={() => void refetchItem()}
                    className="mb-4"
                  />
                )}
                <div className="space-y-3">
                  <div>
                    <h1 className="article-typography text-[28px] font-bold leading-[1.3]">
                      {viewMode === "zh" && translatedTitle
                        ? translatedTitle
                        : displayedArticle.title}
                    </h1>
                    {viewMode === "bilingual" && translatedTitle && (
                      <p className="article-typography mt-1.5 text-[22px] font-semibold leading-[1.35] text-foreground/75">
                        {translatedTitle}
                      </p>
                    )}
                  </div>
                  <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm">
                    {displayedArticle.feed_id > 0 ? (
                      <button
                        type="button"
                        onClick={handleOpenFeed}
                        className="flex max-w-48 items-center gap-1.5 rounded bg-muted px-2 py-1 text-xs font-medium text-muted-foreground transition-colors hover:bg-accent"
                      >
                        {feed && (
                          <FeedFavicon
                            src={getFaviconUrl(feed.link, feed.site_url)}
                            className="h-3.5 w-3.5 rounded-sm"
                          />
                        )}
                        <span className="truncate hover:underline">
                          {feed?.name ?? bookmark?.feed_name ?? t("common.unknown")}
                        </span>
                      </button>
                    ) : (
                      <span className="flex max-w-48 items-center gap-1.5 rounded bg-muted px-2 py-1 text-xs font-medium text-muted-foreground">
                        <span className="truncate">
                          {bookmark?.feed_name ?? t("common.unknown")}
                        </span>
                      </span>
                    )}
                    <span className="text-muted-foreground">
                      {formatDate(displayedArticle.pub_date)}
                    </span>
                    {safeArticleLink ? (
                      <a
                        href={safeArticleLink}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="truncate text-primary hover:underline"
                      >
                        {getLinkDomain(safeArticleLink)}
                      </a>
                    ) : null}
                  </div>
                </div>

                {isSummaryOpen &&
                  (summaryStatus === "summarizing" || hasSummary) && (
                    <section
                      aria-label={t("article.summary.title")}
                      className="mt-5 rounded-xl border border-border/80 bg-muted/45 px-4 py-3.5 sm:px-5"
                    >
                      <div className="mb-2 flex items-center justify-between gap-2">
                        <div className="flex items-center gap-2 text-xs font-semibold text-foreground/85">
                          <Sparkles className="h-3.5 w-3.5 text-primary" />
                          <span>{t("article.summary.title")}</span>
                          {summaryModel && (
                            <span className="font-normal text-muted-foreground">
                              · {summaryModel}
                            </span>
                          )}
                          {summaryStatus === "summarizing" && (
                            <Loader2 className="h-3 w-3 animate-spin text-muted-foreground" />
                          )}
                        </div>
                        <div className="flex items-center gap-1">
                          {summaryStatus !== "summarizing" && (
                            <Button
                              variant="ghost"
                              size="icon-sm"
                              onClick={() => void startSummary({ force: true })}
                              aria-label={t("article.summary.resummarize")}
                              title={t("article.summary.resummarize")}
                              className="h-7 w-7 text-muted-foreground"
                            >
                              <RotateCw
                                className="h-3.5 w-3.5"
                                aria-hidden="true"
                              />
                            </Button>
                          )}
                          <Button
                            variant="ghost"
                            size="icon-sm"
                            onClick={() => setSummaryOpen(false)}
                            aria-label={t("article.summary.collapse")}
                            title={t("article.summary.collapse")}
                            className="h-7 w-7 text-muted-foreground"
                          >
                            <ChevronUp
                              className="h-3.5 w-3.5"
                              aria-hidden="true"
                            />
                          </Button>
                        </div>
                      </div>
                      {renderedSummaryHtml ? (
                        <div
                          className="article-typography rill-summary-content space-y-2 text-[14.5px] leading-[1.68] text-foreground/90"
                          dangerouslySetInnerHTML={{
                            __html: renderedSummaryHtml,
                          }}
                        />
                      ) : (
                        <p className="text-sm text-muted-foreground">
                          {t("article.summary.summarizing")}
                        </p>
                      )}
                    </section>
                  )}

                <div
                  className="typeset typeset-article mt-6 min-w-0 max-w-none"
                  dangerouslySetInnerHTML={{
                    __html: renderedHtml,
                  }}
                />
              </article>
            </ScrollArea>

            {/* Footer - Navigation */}
            <div className="flex items-center justify-between border-t px-4 py-3 sm:px-6">
              <Button
                variant="outline"
                size="sm"
                onClick={goToPrevious}
                disabled={!hasPrevious()}
                className="h-auto gap-1.5 px-3 py-2 text-[13px] font-medium text-muted-foreground"
              >
                <ChevronLeft className="h-4 w-4" />
                {t("common.previous")}
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={goToNext}
                disabled={!hasNext()}
                className="h-auto gap-1.5 px-3 py-2 text-[13px] font-medium text-muted-foreground"
              >
                {t("common.next")}
                <ChevronRight className="h-4 w-4" />
              </Button>
            </div>
          </div>
        ) : selectedArticleId !== null &&
          (isItemLoading || (shouldFetchArticle && isItemError)) ? (
          <div className="flex h-full flex-col">
            <div className="flex items-center justify-end border-b px-4 py-3 sm:px-6">
              <SheetTitle className="sr-only">
                {isItemLoading
                  ? t("article.list.loading")
                  : t("article.detail.loadFailed")}
              </SheetTitle>
              <Button
                variant="ghost"
                size="icon-sm"
                onClick={() => setSelectedArticle(null)}
                aria-label={t("common.cancel")}
              >
                <X className="h-[18px] w-[18px] text-muted-foreground" />
              </Button>
            </div>
            <div className="flex flex-1 items-center justify-center p-6">
              {isItemLoading ? (
                <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
              ) : (
                <QueryErrorState
                  message={t("article.detail.loadFailed")}
                  onRetry={() => void refetchItem()}
                />
              )}
            </div>
          </div>
        ) : null}
      </SheetContent>
    </Sheet>
  );
}

function TranslationModeSwitcher({
  value,
  onChange,
}: {
  value: ArticleViewMode;
  onChange: (mode: ArticleViewMode) => void;
}) {
  const { t } = useI18n();
  const options: Array<{ mode: ArticleViewMode; label: string }> = [
    { mode: "original", label: t("article.translation.mode.original") },
    { mode: "bilingual", label: t("article.translation.mode.bilingual") },
    { mode: "zh", label: t("article.translation.mode.zh") },
  ];

  return (
    <div
      role="group"
      aria-label={t("article.action.translate")}
      className="inline-flex h-[30px] items-center rounded-md border border-input bg-muted/40 p-0.5"
    >
      {options.map((option) => {
        const active = value === option.mode;
        return (
          <button
            key={option.mode}
            type="button"
            aria-pressed={active}
            onClick={() => onChange(option.mode)}
            className={cn(
              "rounded-[4px] px-2 py-0.5 text-[12px] font-medium transition-colors",
              active
                ? "bg-background text-foreground shadow-xs"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            {option.label}
          </button>
        );
      })}
    </div>
  );
}
