import { useCallback, useEffect, useMemo, useRef, useState } from "react";
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

interface CachedTranslationState {
  status: TranslationStatus;
  translatedTitle: string;
  blocks: Record<number, string>;
  totalSegments: number;
  receivedSegments: number;
  model: string;
  errorMessage?: string;
}

const sessionTranslations = new Map<string, CachedTranslationState>();
const sessionViewModes = new Map<string, ArticleViewMode>();

function buildArticleKey(article: Item | null): string | null {
  if (!article) return null;
  return `${article.id}:${article.feed_id}:${article.title.length}:${article.content.length}`;
}

export function useArticleTranslation(
  article: Item | null,
  articleLink: string | null,
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
  const bump = useCallback(() => setVersion((v) => v + 1), []);

  const abortRef = useRef<AbortController | null>(null);
  const activeStreamKeyRef = useRef<string | null>(null);

  // Abort any ongoing stream when switching to another article or closing drawer.
  useEffect(() => {
    return () => {
      if (abortRef.current) {
        abortRef.current.abort();
        abortRef.current = null;
      }
      if (activeStreamKeyRef.current) {
        const key = activeStreamKeyRef.current;
        const prev = sessionTranslations.get(key);
        if (prev && prev.status === "translating") {
          sessionTranslations.delete(key);
        }
        activeStreamKeyRef.current = null;
      }
    };
  }, [articleKey]);

  // Check backend DB cache when opening an article not yet present in session cache.
  useEffect(() => {
    if (!article || !articleKey || !prepared) return;
    if (sessionTranslations.has(articleKey)) return;
    if (!article.title.trim() && prepared.blocks.length === 0) return;

    const controller = new AbortController();
    translationAPI
      .lookup(
        { title: article.title, blocks: prepared.blocks },
        controller.signal,
      )
      .then((res) => {
        if (controller.signal.aborted) return;
        const data = res.data;
        if (!data?.found) return;
        const blockMap: Record<number, string> = {};
        for (const seg of data.blocks ?? []) {
          blockMap[seg.id] = seg.text;
        }
        const total =
          prepared.blocks.length + (article.title.trim() ? 1 : 0);
        sessionTranslations.set(articleKey, {
          status: "ready",
          translatedTitle: data.translated_title ?? "",
          blocks: blockMap,
          totalSegments: total,
          receivedSegments:
            Object.keys(blockMap).length +
            (data.translated_title?.trim() ? 1 : 0),
          model: data.model ?? "",
        });
        bump();
      })
      .catch(() => {
        // Cache lookup failure stays silent until the user triggers translation.
      });

    return () => {
      controller.abort();
    };
  }, [article, articleKey, prepared, bump]);

  const currentState: CachedTranslationState = (articleKey
    ? sessionTranslations.get(articleKey)
    : undefined) ?? {
    status: "idle",
    translatedTitle: "",
    blocks: {},
    totalSegments: 0,
    receivedSegments: 0,
    model: "",
  };

  const viewMode: ArticleViewMode =
    (articleKey ? sessionViewModes.get(articleKey) : undefined) ?? "original";

  const setViewMode = useCallback(
    (mode: ArticleViewMode) => {
      if (!articleKey) return;
      sessionViewModes.set(articleKey, mode);
      bump();
    },
    [articleKey, bump],
  );

  const cancelTranslation = useCallback(() => {
    if (abortRef.current) {
      abortRef.current.abort();
      abortRef.current = null;
    }
    if (articleKey) {
      const existing = sessionTranslations.get(articleKey);
      if (existing && existing.status === "translating") {
        sessionTranslations.delete(articleKey);
        sessionViewModes.set(articleKey, "original");
        bump();
      }
    }
    activeStreamKeyRef.current = null;
  }, [articleKey, bump]);

  const startTranslation = useCallback(
    async (options?: { force?: boolean }) => {
      if (!article || !articleKey || !prepared) return;

      const force = options?.force ?? false;
      const existing = sessionTranslations.get(articleKey);
      if (!force && existing?.status === "ready") {
        sessionViewModes.set(articleKey, preferredMode);
        bump();
        return;
      }

      if (!settingsData?.translation_api_key_set && existing?.status !== "ready") {
        toast.info(t("article.translation.missingConfig"));
        setSettingsOpen(true, "translation");
        return;
      }

      if (abortRef.current) {
        abortRef.current.abort();
      }

      const controller = new AbortController();
      abortRef.current = controller;
      activeStreamKeyRef.current = articleKey;

      const totalSegments =
        prepared.blocks.length + (article.title.trim() ? 1 : 0);

      const state: CachedTranslationState = {
        status: "translating",
        translatedTitle: "",
        blocks: {},
        totalSegments,
        receivedSegments: 0,
        model: "",
      };
      sessionTranslations.set(articleKey, state);
      sessionViewModes.set(articleKey, preferredMode);
      bump();

      try {
        await translationAPI.stream(
          {
            title: article.title,
            blocks: prepared.blocks,
            force,
          },
          {
            onMeta: (meta) => {
              state.totalSegments = meta.total || totalSegments;
              state.model = meta.model;
              bump();
            },
            onSegment: ({ key, text }) => {
              if (key === "title") {
                state.translatedTitle = text;
              } else {
                const id = Number.parseInt(key, 10);
                if (!Number.isNaN(id)) {
                  state.blocks = { ...state.blocks, [id]: text };
                }
              }
              state.receivedSegments =
                Object.keys(state.blocks).length +
                (state.translatedTitle.trim() ? 1 : 0);
              bump();
            },
            onDone: (done) => {
              state.status = "ready";
              state.model = done.model || state.model;
              bump();
            },
          },
          controller.signal,
        );

        if (!controller.signal.aborted && state.status === "translating") {
          state.status = "ready";
          bump();
        }
      } catch (error) {
        if (controller.signal.aborted) {
          return;
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
          toast.error(message || t("article.translation.failed"));
          if (state.receivedSegments === 0) {
            sessionTranslations.delete(articleKey);
            sessionViewModes.set(articleKey, "original");
          }
        }
        bump();
      } finally {
        if (abortRef.current === controller) {
          abortRef.current = null;
        }
        if (activeStreamKeyRef.current === articleKey) {
          activeStreamKeyRef.current = null;
        }
      }
    },
    [
      article,
      articleKey,
      prepared,
      preferredMode,
      settingsData?.translation_api_key_set,
      setSettingsOpen,
      t,
      bump,
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

  const hasTranslatableContent =
    Boolean(article?.title.trim()) || (prepared?.blocks.length ?? 0) > 0;

  return {
    status: currentState.status,
    viewMode,
    setViewMode,
    translatedTitle: currentState.translatedTitle,
    receivedSegments: currentState.receivedSegments,
    totalSegments: currentState.totalSegments,
    renderedHtml,
    hasTranslatableContent,
    hasTranslation:
      currentState.status === "ready" || currentState.receivedSegments > 0,
    startTranslation,
    cancelTranslation,
  };
}
