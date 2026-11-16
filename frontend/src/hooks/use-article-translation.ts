import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type RefObject,
} from "react";
import { toast } from "sonner";
import type { Item } from "@/lib/api";
import { APIError, translationAPI } from "@/lib/api";
import {
  prepareArticleTranslation,
  renderTranslatedArticleHtml,
  type ArticleViewMode,
  type PreparedArticleTranslation,
} from "@/lib/translation";
import { useI18n } from "@/lib/i18n";
import { usePreferencesStore, useUIStore } from "@/store";
import { useSettingsQuery } from "@/queries/settings";

export type TranslationStatus = "idle" | "translating" | "ready" | "error";

const VIEWPORT_NEIGHBOR_BLOCKS = 2;
const MAX_BLOCKS_PER_REQUEST = 20;

interface CachedTranslationState {
  status: TranslationStatus;
  translatedTitle: string;
  blocks: Record<number, string>;
  model: string;
  errorMessage?: string;
}

const sessionTranslations = new Map<string, CachedTranslationState>();
const sessionViewModes = new Map<string, ArticleViewMode>();

function buildArticleKey(article: Item | null): string | null {
  if (!article) return null;
  return `${article.id}:${article.feed_id}:${article.title.length}:${article.content.length}`;
}

function hasAnyTranslation(state: CachedTranslationState): boolean {
  return (
    Boolean(state.translatedTitle.trim()) ||
    Object.keys(state.blocks).length > 0
  );
}

export function useArticleTranslation(
  article: Item | null,
  articleLink: string | null,
  contentRef: RefObject<HTMLElement | null>,
) {
  const { t } = useI18n();
  const preferredMode = usePreferencesStore((s) => s.translationMode);
  const setSettingsOpen = useUIStore((s) => s.setSettingsOpen);
  const { data: settingsData } = useSettingsQuery();

  const articleKey = buildArticleKey(article);

  const prepared: PreparedArticleTranslation | null = useMemo(() => {
    if (!article) return null;
    return prepareArticleTranslation(
      article.content,
      articleLink ?? undefined,
    );
  }, [article, articleLink]);

  const [, setVersion] = useState(0);
  const bump = useCallback(() => setVersion((value) => value + 1), []);

  const controllersRef = useRef<Set<AbortController>>(new Set());
  const inFlightBlocksRef = useRef<Set<number>>(new Set());
  const titleInFlightRef = useRef(false);
  const failedBlocksRef = useRef<Set<number>>(new Set());
  const titleFailedRef = useRef(false);
  const forceNextRequestRef = useRef(false);
  const errorNotifiedRef = useRef(false);

  const abortAll = useCallback(() => {
    for (const controller of controllersRef.current) {
      controller.abort();
    }
    controllersRef.current.clear();
    inFlightBlocksRef.current.clear();
    titleInFlightRef.current = false;
  }, []);

  // A closed/switched article keeps its translated blocks in the session cache
  // but returns to original display; reopening must be an explicit action.
  useEffect(() => {
    failedBlocksRef.current.clear();
    titleFailedRef.current = false;
    forceNextRequestRef.current = false;
    errorNotifiedRef.current = false;

    return () => {
      abortAll();
      if (articleKey) {
        const state = sessionTranslations.get(articleKey);
        if (state?.status === "translating") {
          state.status = hasAnyTranslation(state) ? "ready" : "idle";
        }
        sessionViewModes.set(articleKey, "original");
      }
    };
  }, [articleKey, abortAll]);

  const currentState: CachedTranslationState = (articleKey
    ? sessionTranslations.get(articleKey)
    : undefined) ?? {
    status: "idle",
    translatedTitle: "",
    blocks: {},
    model: "",
  };

  const viewMode: ArticleViewMode =
    (articleKey ? sessionViewModes.get(articleKey) : undefined) ?? "original";

  const setViewMode = useCallback(
    (mode: ArticleViewMode) => {
      if (!articleKey) return;
      sessionViewModes.set(articleKey, mode);
      if (mode === "original") {
        abortAll();
        const state = sessionTranslations.get(articleKey);
        if (state?.status === "translating") {
          state.status = hasAnyTranslation(state) ? "ready" : "idle";
        }
      }
      bump();
    },
    [abortAll, articleKey, bump],
  );

  const requestBlocks = useCallback(
    (
      blockIDs: number[],
      translateTitle: boolean,
      force: boolean,
      articleTitle: string,
    ): boolean => {
      if (!articleKey || !prepared) return false;
      const state = sessionTranslations.get(articleKey);
      if (!state) return false;

      const byID = new Map(prepared.blocks.map((block) => [block.id, block]));
      const blocks = blockIDs
        .map((id) => byID.get(id))
        .filter((block): block is NonNullable<typeof block> => Boolean(block));
      if (blocks.length === 0 && !translateTitle) return false;

      const controller = new AbortController();
      controllersRef.current.add(controller);
      for (const block of blocks) {
        inFlightBlocksRef.current.add(block.id);
      }
      if (translateTitle) {
        titleInFlightRef.current = true;
      }

      state.status = "translating";
      state.errorMessage = undefined;
      let sawDone = false;
      bump();

      void (async () => {
        try {
          await translationAPI.translateBlocks(
            {
              title: translateTitle ? articleTitle : "",
              blocks,
              translate_title: translateTitle,
              force,
            },
            {
              onMeta: (meta) => {
                if (controller.signal.aborted) return;
                state.model = meta.model;
                bump();
              },
              onSegment: ({ key, text }) => {
                if (controller.signal.aborted) return;
                if (key === "title") {
                  state.translatedTitle = text;
                } else {
                  const id = Number.parseInt(key, 10);
                  if (!Number.isNaN(id)) {
                    state.blocks = { ...state.blocks, [id]: text };
                  }
                }
                bump();
              },
              onDone: () => {
                sawDone = true;
              },
            },
            controller.signal,
          );
        } catch (error) {
          if (controller.signal.aborted) return;
          for (const block of blocks) {
            failedBlocksRef.current.add(block.id);
          }
          if (translateTitle) {
            titleFailedRef.current = true;
          }
          const message =
            error instanceof APIError || error instanceof Error
              ? error.message
              : t("article.translation.failed");
          if (
            error instanceof APIError &&
            error.status === 400 &&
            message.toLowerCase().includes("api key")
          ) {
            toast.error(t("article.translation.missingConfig"));
            setSettingsOpen(true, "translation");
            sessionTranslations.delete(articleKey);
            sessionViewModes.set(articleKey, "original");
          } else {
            state.status = "error";
            state.errorMessage = message;
            if (!errorNotifiedRef.current) {
              errorNotifiedRef.current = true;
              toast.error(message || t("article.translation.failed"));
            }
          }
          bump();
          return;
        } finally {
          for (const block of blocks) {
            inFlightBlocksRef.current.delete(block.id);
          }
          if (translateTitle) {
            titleInFlightRef.current = false;
          }
          controllersRef.current.delete(controller);
        }

        if (controller.signal.aborted) return;
        if (!sawDone) {
          for (const block of blocks) {
            failedBlocksRef.current.add(block.id);
          }
          if (translateTitle) {
            titleFailedRef.current = true;
          }
          state.status = "error";
          state.errorMessage = t("article.translation.failed");
          bump();
          return;
        }

        if (controllersRef.current.size === 0 && state.status !== "error") {
          state.status = hasAnyTranslation(state) ? "ready" : "idle";
          bump();
        }
      })();
      return true;
    },
    [articleKey, bump, prepared, setSettingsOpen, t],
  );

  const scanVisibleBlocks = useCallback(() => {
    if (!article || !articleKey || !prepared) return;
    const state = sessionTranslations.get(articleKey);
    if (!state || state.status === "error" || state.status === "idle") {
      return;
    }
    const mode = sessionViewModes.get(articleKey) ?? "original";
    if (mode === "original") return;

    const root = contentRef.current;
    if (!root) return;
    const viewport = root.closest<HTMLElement>(
      '[data-slot="scroll-area-viewport"]',
    );
    const viewportRect = (viewport ?? document.documentElement).getBoundingClientRect();
    const blockIndex = new Map(
      prepared.blocks.map((block, index) => [block.id, index]),
    );
    const visibleIndexes: number[] = [];
    for (const element of root.querySelectorAll<HTMLElement>("[data-rill-tb]")) {
      const rect = element.getBoundingClientRect();
      if (rect.bottom < viewportRect.top || rect.top > viewportRect.bottom) {
        continue;
      }
      const id = Number.parseInt(
        element.getAttribute("data-rill-tb") ?? "",
        10,
      );
      const index = blockIndex.get(id);
      if (index !== undefined) {
        visibleIndexes.push(index);
      }
    }

    let minIndex = Number.POSITIVE_INFINITY;
    let maxIndex = Number.NEGATIVE_INFINITY;
    for (const index of visibleIndexes) {
      minIndex = Math.min(minIndex, index);
      maxIndex = Math.max(maxIndex, index);
    }
    if (!Number.isFinite(minIndex)) {
      minIndex = 0;
      maxIndex = -1;
    }

    const start = Math.max(0, minIndex - VIEWPORT_NEIGHBOR_BLOCKS);
    const end = Math.min(
      prepared.blocks.length - 1,
      maxIndex + VIEWPORT_NEIGHBOR_BLOCKS,
    );
    const missing: number[] = [];
    for (let index = start; index <= end; index++) {
      const block = prepared.blocks[index];
      if (
        block &&
        !state.blocks[block.id] &&
        !inFlightBlocksRef.current.has(block.id) &&
        !failedBlocksRef.current.has(block.id)
      ) {
        missing.push(block.id);
      }
    }

    const translateTitle =
      !state.translatedTitle.trim() &&
      !titleInFlightRef.current &&
      !titleFailedRef.current;
    if (missing.length === 0 && !translateTitle) {
      if (controllersRef.current.size === 0 && state.status === "translating") {
        state.status = hasAnyTranslation(state) ? "ready" : "idle";
        bump();
      }
      return;
    }

    const force = forceNextRequestRef.current;
    if (force) {
      forceNextRequestRef.current = false;
    }

    let titleForFirstChunk = translateTitle;
    const chunks =
      missing.length === 0
        ? [[]]
        : Array.from(
            { length: Math.ceil(missing.length / MAX_BLOCKS_PER_REQUEST) },
            (_, index) =>
              missing.slice(
                index * MAX_BLOCKS_PER_REQUEST,
                (index + 1) * MAX_BLOCKS_PER_REQUEST,
              ),
          );
    for (const chunk of chunks) {
      requestBlocks(
        chunk,
        titleForFirstChunk,
        force,
        article.title,
      );
      titleForFirstChunk = false;
    }
  }, [article, articleKey, bump, contentRef, prepared, requestBlocks]);

  useEffect(() => {
    if (!article || viewMode === "original") return;
    const root = contentRef.current;
    if (!root) return;
    const viewport = root.closest<HTMLElement>(
      '[data-slot="scroll-area-viewport"]',
    );
    let frame: number | null = null;
    const scheduleScan = () => {
      if (frame !== null) return;
      frame = window.requestAnimationFrame(() => {
        frame = null;
        scanVisibleBlocks();
      });
    };
    scanVisibleBlocks();
    viewport?.addEventListener("scroll", scheduleScan, { passive: true });
    window.addEventListener("resize", scheduleScan);
    return () => {
      viewport?.removeEventListener("scroll", scheduleScan);
      window.removeEventListener("resize", scheduleScan);
      if (frame !== null) {
        window.cancelAnimationFrame(frame);
      }
    };
  }, [
    article,
    contentRef,
    currentState.blocks,
    currentState.translatedTitle,
    scanVisibleBlocks,
    viewMode,
  ]);

  const cancelTranslation = useCallback(() => {
    abortAll();
    if (articleKey) {
      const state = sessionTranslations.get(articleKey);
      if (state) {
        state.status = hasAnyTranslation(state) ? "ready" : "idle";
      }
      sessionViewModes.set(articleKey, "original");
    }
    bump();
  }, [abortAll, articleKey, bump]);

  const startTranslation = useCallback(
    (options?: { force?: boolean }) => {
      if (!article || !articleKey || !prepared) return;
      const force = options?.force ?? false;
      const existing = sessionTranslations.get(articleKey);

      if (!force && existing && hasAnyTranslation(existing)) {
        sessionViewModes.set(articleKey, preferredMode);
        bump();
        return;
      }

      if (!settingsData?.translation_api_key_set) {
        toast.info(t("article.translation.missingConfig"));
        setSettingsOpen(true, "translation");
        return;
      }

      if (force) {
        abortAll();
      }

      const state: CachedTranslationState = force || !existing
        ? {
            status: "translating",
            translatedTitle: "",
            blocks: {},
            model: "",
          }
        : existing;
      state.status = "translating";
      state.errorMessage = undefined;
      sessionTranslations.set(articleKey, state);
      sessionViewModes.set(articleKey, preferredMode);
      failedBlocksRef.current.clear();
      titleFailedRef.current = false;
      errorNotifiedRef.current = false;
      forceNextRequestRef.current = force;
      bump();
    },
    [
      abortAll,
      article,
      articleKey,
      bump,
      preferredMode,
      prepared,
      setSettingsOpen,
      settingsData?.translation_api_key_set,
      t,
    ],
  );

  const renderedHtml = useMemo(() => {
    if (!prepared) return "";
    return renderTranslatedArticleHtml(
      prepared,
      currentState.blocks,
      viewMode,
    );
  }, [prepared, currentState.blocks, viewMode]);

  const receivedSegments =
    Object.keys(currentState.blocks).length +
    (currentState.translatedTitle.trim() ? 1 : 0);
  const hasTranslatableContent =
    Boolean(article?.title.trim()) || (prepared?.blocks.length ?? 0) > 0;

  return {
    status: currentState.status,
    viewMode,
    setViewMode,
    translatedTitle: currentState.translatedTitle,
    receivedSegments,
    totalSegments: 0,
    renderedHtml,
    hasTranslatableContent,
    hasTranslation:
      currentState.status === "ready" || receivedSegments > 0,
    startTranslation,
    cancelTranslation,
  };
}
