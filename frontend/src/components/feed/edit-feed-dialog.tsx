import { useEffect, useMemo, useRef, useState } from "react";
import { AlertCircle, ChevronDown, Eye, Loader2, Save, Trash2, X } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Switch } from "@/components/ui/switch";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useUIStore } from "@/store";
import { useGroups } from "@/queries/groups";
import { useUpdateFeed, useDeleteFeed, usePreviewFeedExtraction } from "@/queries/feeds";
import { useAppInfo } from "@/queries/app";
import { processArticleContent } from "@/lib/content";
import { feedSelectorUpdate } from "@/lib/feed-extraction";
import type { Feed, PreviewFeedExtractionResponse, UpdateFeedRequest } from "@/lib/api";
import { toast } from "sonner";
import { useI18n } from "@/lib/i18n";
import {
  DEFAULT_PULL_INTERVAL,
  FALLBACK_REFRESH_INTERVALS,
  formatInterval,
  intervalOptions,
} from "@/lib/refresh-interval";
import { cn } from "@/lib/utils";
import { useIsMobile } from "@/hooks/use-mobile";

export function EditFeedDialog() {
  const { isEditFeedOpen, editingFeed, setEditFeedOpen } = useUIStore();

  if (!editingFeed) {
    return null;
  }

  return (
    <EditFeedDialogForm
      key={`${editingFeed.id}:${editingFeed.updated_at}`}
      open={isEditFeedOpen}
      editingFeed={editingFeed}
      onOpenChange={(open) => setEditFeedOpen(open)}
      onClose={() => setEditFeedOpen(false)}
    />
  );
}

interface EditFeedDialogFormProps {
  open: boolean;
  editingFeed: Feed;
  onOpenChange: (open: boolean) => void;
  onClose: () => void;
}

function EditFeedDialogForm({
  open,
  editingFeed,
  onOpenChange,
  onClose,
}: EditFeedDialogFormProps) {
  const { t } = useI18n();
  const { data: groups = [] } = useGroups();
  const updateFeedMutation = useUpdateFeed();
  const deleteFeedMutation = useDeleteFeed();
  const previewMutation = usePreviewFeedExtraction();

  // NULL (or a legacy 0) stays "untouched" (empty): the Select shows the
  // live global fallback, and an untouched save sends nothing.
  const storedInterval = editingFeed.refresh_interval_seconds;

  const [url, setUrl] = useState(editingFeed.link);
  const [name, setName] = useState(editingFeed.name);
  const [groupId, setGroupId] = useState<string>(editingFeed.group_id.toString());
  const [proxy, setProxy] = useState(editingFeed.proxy ?? "");
  const [contentSelector, setContentSelector] = useState(
    editingFeed.content_selector ?? "",
  );
  const [excludeSelector, setExcludeSelector] = useState(
    editingFeed.exclude_selector ?? "",
  );
  const [previewURL, setPreviewURL] = useState("");
  const [previewResult, setPreviewResult] =
    useState<PreviewFeedExtractionResponse | null>(null);
  const [previewError, setPreviewError] = useState<string | null>(null);
  const [isPreviewing, setIsPreviewing] = useState(false);
  const previewRequestID = useRef(0);
  const previewAbortRef = useRef<AbortController | null>(null);
  const [suspended, setSuspended] = useState(editingFeed.suspended);
  const [refreshInterval, setRefreshInterval] = useState<string>(
    storedInterval && storedInterval > 0 ? storedInterval.toString() : "",
  );
  const [isAdvancedOpen, setIsAdvancedOpen] = useState(
    !!editingFeed.proxy ||
      storedInterval != null ||
      !!editingFeed.content_selector ||
      !!editingFeed.exclude_selector,
  );
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [isDeleteOpen, setIsDeleteOpen] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);
  const [isMobileErrorTooltipOpen, setIsMobileErrorTooltipOpen] =
    useState(false);
  const urlInputRef = useRef<HTMLInputElement>(null);
  const isMobile = useIsMobile();
  const groupItems = groups.map((group) => ({
    value: group.id.toString(),
    label: group.name,
  }));

  const { data: appInfo, isLoading: appInfoLoading } = useAppInfo();
  const globalPullInterval = appInfo?.pull_interval ?? DEFAULT_PULL_INTERVAL;
  const options = intervalOptions(
    globalPullInterval,
    appInfo?.allowed_refresh_intervals ?? FALLBACK_REFRESH_INTERVALS,
  );

  const renderedPreview = useMemo(
    () =>
      previewResult
        ? processArticleContent(previewResult.content, previewResult.url)
        : "",
    [previewResult],
  );

  useEffect(() => {
    return () => {
      previewAbortRef.current?.abort();
    };
  }, []);

  const resetPreview = () => {
    previewRequestID.current += 1;
    previewAbortRef.current?.abort();
    previewAbortRef.current = null;
    previewMutation.reset();
    setIsPreviewing(false);
    setPreviewResult(null);
    setPreviewError(null);
  };

  const updatePreviewInput = (
    setter: (value: string) => void,
    value: string,
  ) => {
    setter(value);
    resetPreview();
  };

  const handlePreview = async () => {
    const trimmedURL = previewURL.trim();
    const trimmedContentSelector = contentSelector.trim();
    if (!trimmedURL) {
      setPreviewError(t("feed.edit.previewUrlRequired"));
      return;
    }
    if (!trimmedContentSelector) {
      setPreviewError(t("feed.edit.contentSelectorRequired"));
      return;
    }

    previewAbortRef.current?.abort();
    const requestID = previewRequestID.current + 1;
    previewRequestID.current = requestID;
    const controller = new AbortController();
    previewAbortRef.current = controller;
    setIsPreviewing(true);
    setPreviewResult(null);
    setPreviewError(null);

    try {
      const result = await previewMutation.mutateAsync({
        feedId: editingFeed.id,
        data: {
          url: trimmedURL,
          content_selector: trimmedContentSelector,
          exclude_selector: excludeSelector.trim(),
          proxy: proxy.trim(),
        },
        signal: controller.signal,
      });
      if (previewRequestID.current !== requestID || controller.signal.aborted) {
        return;
      }
      setPreviewResult(result);
    } catch (error) {
      if (previewRequestID.current !== requestID || controller.signal.aborted) {
        return;
      }
      setPreviewError(
        error instanceof Error
          ? error.message
          : t("feed.edit.previewFailed"),
      );
    } finally {
      if (previewRequestID.current === requestID) {
        setIsPreviewing(false);
      }
    }
  };

  const handleClose = () => {
    onClose();
  };

  const handleSubmit = async () => {
    if (!editingFeed) return;

    if (!url.trim()) {
      toast.error(t("feed.toast.enterUrl"));
      return;
    }

    if (!name.trim()) {
      toast.error(t("feed.toast.enterName"));
      return;
    }

    setIsSubmitting(true);
    try {
      const request: UpdateFeedRequest = {};

      if (url.trim() !== editingFeed.link) {
        request.link = url.trim();
      }

      if (name.trim() !== editingFeed.name) {
        request.name = name.trim();
      }

      const newGroupId = parseInt(groupId, 10);
      if (newGroupId !== editingFeed.group_id) {
        request.group_id = newGroupId;
      }

      if (suspended !== editingFeed.suspended) {
        request.suspended = suspended;
      }

      const newProxy = proxy.trim() || undefined;
      if (newProxy !== editingFeed.proxy) {
        request.proxy = newProxy;
      }

      const contentSelectorUpdate = feedSelectorUpdate(
        editingFeed.content_selector,
        contentSelector,
      );
      if (contentSelectorUpdate !== undefined) {
        request.content_selector = contentSelectorUpdate;
      }

      const excludeSelectorUpdate = feedSelectorUpdate(
        editingFeed.exclude_selector,
        excludeSelector,
      );
      if (excludeSelectorUpdate !== undefined) {
        request.exclude_selector = excludeSelectorUpdate;
      }

      // The global-interval entry means "follow the global setting" and is
      // stored as NULL (cleared via 0); anything else is an explicit override.
      const stored = editingFeed.refresh_interval_seconds;
      const selected = parseInt(refreshInterval || String(globalPullInterval), 10);
      if (selected !== (stored && stored > 0 ? stored : globalPullInterval)) {
        request.refresh_interval_seconds =
          selected === globalPullInterval ? 0 : selected;
      }

      if (Object.keys(request).length === 0) {
        toast.info(t("feed.edit.noChanges"));
        handleClose();
        return;
      }

      await updateFeedMutation.mutateAsync({ id: editingFeed.id, ...request });
      toast.success(t("feed.toast.updated"));
      handleClose();
    } catch {
      toast.error(t("feed.toast.updateFailed"));
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleDelete = async () => {
    if (!editingFeed) return;

    setIsDeleting(true);
    try {
      await deleteFeedMutation.mutateAsync(editingFeed.id);
      toast.success(t("feed.toast.unsubscribed", { name: editingFeed.name }));
      setIsDeleteOpen(false);
      handleClose();
    } catch {
      toast.error(t("feed.toast.unsubscribeFailed"));
    } finally {
      setIsDeleting(false);
    }
  };

  return (
    <>
      <Dialog
        open={open}
        onOpenChange={onOpenChange}
      >
        <DialogContent
          className="flex max-h-[85vh] w-full max-w-[480px] flex-col gap-0 overflow-hidden p-0"
          showCloseButton={false}
          initialFocus={urlInputRef}
        >
          {/* Header */}
          <DialogHeader className="flex shrink-0 flex-row items-center justify-between border-b px-5 py-4">
            <DialogTitle className="flex items-center gap-1.5 text-base font-semibold">
              <span>{t("feed.edit.title")}</span>
              {editingFeed?.fetch_state.last_error && (
                <Tooltip
                  open={isMobile ? isMobileErrorTooltipOpen : undefined}
                  onOpenChange={(open) => {
                    if (!isMobile || open) return;
                    setIsMobileErrorTooltipOpen(false);
                  }}
                >
                  <TooltipTrigger
                    render={
                      <button
                        type="button"
                        aria-label={t("feeds.status.error")}
                        onClick={() => {
                          if (!isMobile) return;
                          setIsMobileErrorTooltipOpen((open) => !open);
                        }}
                        className="inline-flex cursor-help items-center text-destructive"
                      />
                    }
                  >
                    <AlertCircle className="h-4 w-4" />
                  </TooltipTrigger>
                  <TooltipContent
                    side="bottom"
                    className="max-w-sm whitespace-normal break-words"
                  >
                    {editingFeed.fetch_state.last_error.trim()}
                  </TooltipContent>
                </Tooltip>
              )}
            </DialogTitle>
            <Button variant="ghost" size="icon-sm" onClick={handleClose}>
              <span className="sr-only">{t("common.cancel")}</span>
              <X className="h-[18px] w-[18px] text-muted-foreground" />
            </Button>
          </DialogHeader>

          {/* Form Content */}
          <div className="min-h-0 flex-1 space-y-4 overflow-y-auto p-5">
            {/* URL Section */}
            <div className="space-y-1.5">
              <label htmlFor="edit-feed-url" className="text-[13px] font-medium">
                {t("feed.add.urlLabel")}
              </label>
              <Input
                ref={urlInputRef}
                id="edit-feed-url"
                name="feed-url"
                type="url"
                inputMode="url"
                placeholder={t("feed.add.urlPlaceholder")}
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                className="h-10"
                autoComplete="off"
                spellCheck={false}
              />
            </div>

            {/* Name Section */}
            <div className="space-y-1.5">
              <label htmlFor="edit-feed-name" className="text-[13px] font-medium">
                {t("feed.add.nameLabel")}
              </label>
              <Input
                id="edit-feed-name"
                name="feed-name"
                placeholder={t("feed.add.namePlaceholder")}
                value={name}
                onChange={(e) => setName(e.target.value)}
                className="h-10"
                autoComplete="off"
              />
            </div>

            {/* Group Section */}
            <div className="space-y-1.5">
              <label className="text-[13px] font-medium" id="edit-feed-group-label">
                {t("feed.add.groupLabel")}
              </label>
              <Select
                items={groupItems}
                value={groupId || null}
                onValueChange={(v) => {
                  if (v) setGroupId(v);
                }}
              >
                <SelectTrigger className="h-10" aria-labelledby="edit-feed-group-label">
                  <SelectValue placeholder={t("feed.add.groupPlaceholder")} />
                </SelectTrigger>
                <SelectContent>
                  {groupItems.map((group) => (
                    <SelectItem key={group.value} value={group.value}>
                      {group.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {/* Suspended Toggle */}
            <div className="flex items-center justify-between">
              <div>
                <label
                  htmlFor="edit-feed-suspended"
                  className="text-[13px] font-medium"
                >
                  {t("feed.edit.suspendLabel")}
                </label>
                <p className="text-xs text-muted-foreground">
                  {t("feed.edit.suspendDescription")}
                </p>
              </div>
              <Switch
                id="edit-feed-suspended"
                checked={suspended}
                onCheckedChange={setSuspended}
              />
            </div>

            {/* Advanced Section */}
            <Collapsible open={isAdvancedOpen} onOpenChange={setIsAdvancedOpen}>
              <CollapsibleTrigger className="flex w-full items-center gap-1.5 text-[13px] font-medium text-muted-foreground">
                <ChevronDown
                  className={cn(
                    "h-3.5 w-3.5 transition-transform",
                    isAdvancedOpen && "rotate-180",
                  )}
                />
                {t("feed.add.advanced")}
              </CollapsibleTrigger>
              <CollapsibleContent className="space-y-4 pl-5 pt-3">
                <div className="space-y-1.5">
                  <label className="text-[13px] font-medium" id="edit-feed-refresh-label">
                    {t("feed.add.refreshFrequencyLabel")}
                  </label>
                  <Select
                    items={options.map((seconds) => ({
                      value: seconds.toString(),
                      label: formatInterval(seconds),
                    }))}
                    disabled={appInfoLoading}
                    value={refreshInterval || String(globalPullInterval)}
                    onValueChange={(v) => v && setRefreshInterval(v)}
                  >
                    <SelectTrigger className="h-10" aria-labelledby="edit-feed-refresh-label">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {options.map((seconds) => (
                        <SelectItem key={seconds} value={seconds.toString()}>
                          {formatInterval(seconds)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-1.5">
                  <label htmlFor="edit-feed-proxy" className="text-[13px] font-medium">
                    {t("feed.add.proxyLabel")}
                  </label>
                  <Input
                    id="edit-feed-proxy"
                    name="feed-proxy"
                    type="url"
                    inputMode="url"
                    placeholder={t("feed.add.proxyPlaceholder")}
                    value={proxy}
                    onChange={(e) =>
                      updatePreviewInput(setProxy, e.target.value)
                    }
                    className="h-10"
                    autoComplete="off"
                    spellCheck={false}
                  />
                  <p className="text-xs text-muted-foreground">
                    {t("feed.add.proxyHint")}
                  </p>
                </div>
                <div className="space-y-1.5">
                  <label
                    htmlFor="edit-feed-content-selector"
                    className="text-[13px] font-medium"
                  >
                    {t("feed.edit.contentSelectorLabel")}
                  </label>
                  <Input
                    id="edit-feed-content-selector"
                    name="feed-content-selector"
                    placeholder={t("feed.edit.contentSelectorPlaceholder")}
                    value={contentSelector}
                    onChange={(e) =>
                      updatePreviewInput(setContentSelector, e.target.value)
                    }
                    className="h-10 font-mono text-sm"
                    autoComplete="off"
                    spellCheck={false}
                  />
                  <p className="text-xs text-muted-foreground">
                    {t("feed.edit.contentSelectorHint")}
                  </p>
                </div>
                <div className="space-y-1.5">
                  <label
                    htmlFor="edit-feed-exclude-selector"
                    className="text-[13px] font-medium"
                  >
                    {t("feed.edit.excludeSelectorLabel")}
                  </label>
                  <Input
                    id="edit-feed-exclude-selector"
                    name="feed-exclude-selector"
                    placeholder={t("feed.edit.excludeSelectorPlaceholder")}
                    value={excludeSelector}
                    onChange={(e) =>
                      updatePreviewInput(setExcludeSelector, e.target.value)
                    }
                    className="h-10 font-mono text-sm"
                    autoComplete="off"
                    spellCheck={false}
                  />
                  <p className="text-xs text-muted-foreground">
                    {t("feed.edit.excludeSelectorHint")}
                  </p>
                </div>
                <div className="space-y-1.5 border-t pt-4">
                  <label
                    htmlFor="edit-feed-preview-url"
                    className="text-[13px] font-medium"
                  >
                    {t("feed.edit.previewUrlLabel")}
                  </label>
                  <Input
                    id="edit-feed-preview-url"
                    name="feed-preview-url"
                    type="url"
                    inputMode="url"
                    placeholder={t("feed.edit.previewUrlPlaceholder")}
                    value={previewURL}
                    onChange={(e) =>
                      updatePreviewInput(setPreviewURL, e.target.value)
                    }
                    className="h-10"
                    autoComplete="off"
                    spellCheck={false}
                  />
                  <div className="flex items-start gap-2">
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() => void handlePreview()}
                      disabled={
                        isPreviewing ||
                        !previewURL.trim() ||
                        !contentSelector.trim()
                      }
                    >
                      {isPreviewing ? (
                        <Loader2 className="h-4 w-4 animate-spin" />
                      ) : (
                        <Eye className="h-4 w-4" />
                      )}
                      {isPreviewing
                        ? t("feed.edit.previewing")
                        : t("feed.edit.previewAction")}
                    </Button>
                    <p className="pt-1 text-xs text-muted-foreground">
                      {t("feed.edit.previewHint")}
                    </p>
                  </div>
                  {previewError && (
                    <p
                      role="alert"
                      className="break-words text-xs text-destructive"
                    >
                      {previewError}
                    </p>
                  )}
                  {previewResult && (
                    <div className="space-y-2">
                      <p className="text-[13px] font-medium">
                        {t("feed.edit.previewResult")}
                      </p>
                      <div className="max-h-80 overflow-y-auto rounded-lg border bg-background p-3.5">
                        <div
                          className="typeset typeset-article"
                          dangerouslySetInnerHTML={{
                            __html: renderedPreview,
                          }}
                        />
                      </div>
                    </div>
                  )}
                </div>
              </CollapsibleContent>
            </Collapsible>
          </div>

          {/* Footer */}
          <div className="flex shrink-0 items-center justify-between border-t px-5 py-4">
            <Button
              variant="ghost"
              size="sm"
              className="text-destructive hover:text-destructive hover:bg-destructive/10"
              onClick={() => setIsDeleteOpen(true)}
            >
              <Trash2 className="mr-1.5 h-3.5 w-3.5" />
              {t("feed.edit.unsubscribe")}
            </Button>
            <div className="flex items-center gap-3">
              <Button variant="outline" onClick={handleClose}>
                {t("common.cancel")}
              </Button>
              <Button
                onClick={handleSubmit}
                disabled={isSubmitting || !url.trim() || !name.trim()}
              >
                <Save className="mr-1.5 h-4 w-4" />
                {t("common.save")}
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>

      {/* Delete Confirmation Dialog */}
      <Dialog open={isDeleteOpen} onOpenChange={setIsDeleteOpen}>
        <DialogContent className="sm:max-w-[425px]">
          <DialogHeader>
            <DialogTitle>{t("feed.edit.deleteConfirm.title")}</DialogTitle>
            <DialogDescription>
              {t("feed.edit.deleteConfirm.description", {
                name: editingFeed?.name ?? "",
              })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setIsDeleteOpen(false)}
              disabled={isDeleting}
            >
              {t("common.cancel")}
            </Button>
            <Button
              variant="destructive"
              onClick={handleDelete}
              disabled={isDeleting}
            >
              {isDeleting ? t("common.deleting") : t("feed.edit.unsubscribe")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
