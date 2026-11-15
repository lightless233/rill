import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { toast } from "sonner";
import type { Item } from "@/lib/api";
import { APIError, summaryAPI } from "@/lib/api";
import {
  extractArticleSummaryText,
  renderSafeSummaryHtml,
} from "@/lib/summary";
import { useI18n } from "@/lib/i18n";
import { useUIStore } from "@/store";
import { useSettingsQuery } from "@/queries/settings";

export type SummaryStatus = "idle" | "summarizing" | "ready" | "error";

interface CachedSummaryState {
  status: SummaryStatus;
  summary: string;
  model: string;
}

const sessionSummaries = new Map<string, CachedSummaryState>();
const sessionSummaryOpen = new Map<string, boolean>();

function buildArticleKey(article: Item | null): string | null {
  if (!article) return null;
  return `${article.id}:${article.feed_id}:${article.title.length}:${article.content.length}`;
}

export function useArticleSummary(article: Item | null) {
  const { t } = useI18n();
  const setSettingsOpen = useUIStore((s) => s.setSettingsOpen);
  const { data: settingsData } = useSettingsQuery();

  const articleKey = buildArticleKey(article);
  const sourceText = useMemo(() => {
    if (!article) return "";
    return extractArticleSummaryText(article.content);
  }, [article]);

  const [, setVersion] = useState(0);
  const bump = useCallback(() => setVersion((v) => v + 1), []);

  const abortRef = useRef<AbortController | null>(null);
  const activeStreamKeyRef = useRef<string | null>(null);

  useEffect(() => {
    return () => {
      if (abortRef.current) {
        abortRef.current.abort();
        abortRef.current = null;
      }
      if (activeStreamKeyRef.current) {
        const key = activeStreamKeyRef.current;
        const prev = sessionSummaries.get(key);
        if (prev && prev.status === "summarizing") {
          sessionSummaries.delete(key);
        }
        activeStreamKeyRef.current = null;
      }
    };
  }, [articleKey]);

  // Check backend DB cache when opening an article not yet loaded in session cache.
  useEffect(() => {
    if (!article || !articleKey) return;
    if (sessionSummaries.has(articleKey)) return;
    if (!article.title.trim() && !sourceText) return;

    const controller = new AbortController();
    summaryAPI
      .lookup(
        { title: article.title, content: sourceText },
        controller.signal,
      )
      .then((res) => {
        if (controller.signal.aborted) return;
        const data = res.data;
        if (!data?.found || !data.summary?.trim()) return;
        sessionSummaries.set(articleKey, {
          status: "ready",
          summary: data.summary,
          model: data.model ?? "",
        });
        if (!sessionSummaryOpen.has(articleKey)) {
          sessionSummaryOpen.set(articleKey, true);
        }
        bump();
      })
      .catch(() => {
        // Silent fallback until user explicitly triggers summarization.
      });

    return () => {
      controller.abort();
    };
  }, [article, articleKey, sourceText, bump]);

  const currentState: CachedSummaryState = (articleKey
    ? sessionSummaries.get(articleKey)
    : undefined) ?? {
    status: "idle",
    summary: "",
    model: "",
  };

  const isOpen = articleKey
    ? (sessionSummaryOpen.get(articleKey) ?? false)
    : false;

  const setOpen = useCallback(
    (open: boolean) => {
      if (!articleKey) return;
      sessionSummaryOpen.set(articleKey, open);
      bump();
    },
    [articleKey, bump],
  );

  const cancelSummary = useCallback(() => {
    if (abortRef.current) {
      abortRef.current.abort();
      abortRef.current = null;
    }
    if (articleKey) {
      const existing = sessionSummaries.get(articleKey);
      if (existing && existing.status === "summarizing") {
        sessionSummaries.delete(articleKey);
        sessionSummaryOpen.set(articleKey, false);
        bump();
      }
    }
    activeStreamKeyRef.current = null;
  }, [articleKey, bump]);

  const startSummary = useCallback(
    async (options?: { force?: boolean }) => {
      if (!article || !articleKey) return;
      if (!article.title.trim() && !sourceText) return;

      const force = options?.force ?? false;
      const existing = sessionSummaries.get(articleKey);
      if (!force && existing?.status === "ready") {
        sessionSummaryOpen.set(articleKey, true);
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

      const state: CachedSummaryState = {
        status: "summarizing",
        summary: force ? "" : (existing?.summary ?? ""),
        model: "",
      };
      sessionSummaries.set(articleKey, state);
      sessionSummaryOpen.set(articleKey, true);
      bump();

      try {
        await summaryAPI.stream(
          {
            title: article.title,
            content: sourceText,
            force,
          },
          {
            onMeta: (meta) => {
              state.model = meta.model;
              if (meta.cached) {
                state.summary = "";
              }
              bump();
            },
            onDelta: ({ text }) => {
              state.summary += text;
              bump();
            },
            onDone: (done) => {
              state.status = "ready";
              if (done.summary) {
                state.summary = done.summary;
              }
              state.model = done.model || state.model;
              bump();
            },
          },
          controller.signal,
        );

        if (!controller.signal.aborted && state.status === "summarizing") {
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
            : t("article.summary.failed");
        if (
          error instanceof APIError &&
          error.status === 400 &&
          message.toLowerCase().includes("api key")
        ) {
          toast.error(t("article.translation.missingConfig"));
          setSettingsOpen(true, "translation");
          sessionSummaries.delete(articleKey);
          sessionSummaryOpen.set(articleKey, false);
        } else {
          state.status = "error";
          toast.error(message || t("article.summary.failed"));
          if (!state.summary.trim()) {
            sessionSummaries.delete(articleKey);
            sessionSummaryOpen.set(articleKey, false);
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
      sourceText,
      settingsData?.translation_api_key_set,
      setSettingsOpen,
      t,
      bump,
    ],
  );

  const renderedSummaryHtml = useMemo(
    () => renderSafeSummaryHtml(currentState.summary),
    [currentState.summary],
  );

  const hasSummarizableContent = Boolean(
    article && (article.title.trim() || sourceText),
  );

  return {
    status: currentState.status,
    summaryText: currentState.summary,
    renderedSummaryHtml,
    summaryModel: currentState.model,
    hasSummary: currentState.status === "ready" && Boolean(currentState.summary.trim()),
    hasSummarizableContent,
    isOpen,
    setOpen,
    startSummary,
    cancelSummary,
  };
}
