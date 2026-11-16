import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { Item, ListTranslationMode } from "@/lib/api";
import { translationAPI } from "@/lib/api";
import {
  buildListTranslationInput,
  listTranslationSignature,
  LIST_TRANSLATION_BATCH_SIZE,
  type ListTranslationInput,
  type ListTranslationText,
} from "@/lib/list-translation";

export type ListTranslationStatus =
  | "idle"
  | "translating"
  | "incomplete"
  | "error";

type FailureStatus = "incomplete" | "error";

interface UseListTranslationOptions {
  articles: Item[];
  mode: ListTranslationMode;
  hasKey: boolean;
  scopeKey: string;
}

// Session cache survives list/filter navigation within one document; the
// backend cache remains the cross-reload source of truth.
const translationCache = new Map<string, ListTranslationText>();

export function useListTranslation({
  articles,
  mode,
  hasKey,
  scopeKey,
}: UseListTranslationOptions) {
  const enabled = mode !== "original" && hasKey;

  const requestItems = useMemo(() => {
    if (!enabled) return [];
    return articles
      .map(buildListTranslationInput)
      .filter((item): item is ListTranslationInput => item !== null);
  }, [articles, enabled]);

  const signature = useMemo(
    () => listTranslationSignature(requestItems),
    [requestItems],
  );
  const requestItemsRef = useRef(requestItems);
  useEffect(() => {
    requestItemsRef.current = requestItems;
  }, [requestItems]);

  const [, setVersion] = useState(0);
  const bump = useCallback(() => setVersion((value) => value + 1), []);
  const [status, setStatus] = useState<ListTranslationStatus>("idle");
  const [error, setError] = useState<string | null>(null);
  const [retryToken, setRetryToken] = useState(0);

  // The current list owns its blocked keys and worker count. Changing scope,
  // mode or retry aborts old workers and clears this state so a previous
  // list's failure cannot mark the new list as failed.
  const blockedKeysRef = useRef<Set<string>>(new Set());
  const inFlightKeysRef = useRef<Set<string>>(new Set());
  const controllersRef = useRef<Set<AbortController>>(new Set());
  const activeWorkersRef = useRef(0);
  const failedStatusRef = useRef<FailureStatus | null>(null);
  const lifecycleRef = useRef({ enabled, scopeKey, retryToken });
  const generationRef = useRef(0);

  useEffect(() => {
    const previous = lifecycleRef.current;
    const changed =
      previous.enabled !== enabled ||
      previous.scopeKey !== scopeKey ||
      previous.retryToken !== retryToken;

    if (changed) {
      generationRef.current += 1;
      lifecycleRef.current = { enabled, scopeKey, retryToken };
      blockedKeysRef.current.clear();
      inFlightKeysRef.current.clear();
      activeWorkersRef.current = 0;
      failedStatusRef.current = null;
      void (async () => {
        setStatus("idle");
        setError(null);
      })();
    }

    // These sets are stable containers; workers mutate them in place.
    const controllers = controllersRef.current;
    const inFlightKeys = inFlightKeysRef.current;
    return () => {
      for (const controller of controllers) {
        controller.abort();
      }
      controllers.clear();
      inFlightKeys.clear();
    };
  }, [enabled, scopeKey, retryToken]);

  // A new page claims only items that are not already in flight. Appending
  // items therefore starts another worker instead of aborting the current
  // batch; shared in-flight keys prevent duplicate requests for the same item.
  const startWorker = useCallback(() => {
    if (!enabled) return;

    const pending = requestItemsRef.current.filter(
      (item) =>
        !translationCache.has(item.key) &&
        !blockedKeysRef.current.has(item.key) &&
        !inFlightKeysRef.current.has(item.key),
    );

    if (pending.length === 0) {
      void (async () => {
        if (activeWorkersRef.current === 0) {
          setStatus(failedStatusRef.current ?? "idle");
          if (!failedStatusRef.current) {
            setError(null);
          }
        }
      })();
      return;
    }

    for (const item of pending) {
      inFlightKeysRef.current.add(item.key);
    }

    const claimed = pending;
    const generation = generationRef.current;
    const controller = new AbortController();
    controllersRef.current.add(controller);
    activeWorkersRef.current += 1;

    void (async () => {
      setStatus("translating");
      setError(null);

      try {
        for (
          let start = 0;
          start < claimed.length;
          start += LIST_TRANSLATION_BATCH_SIZE
        ) {
          if (controller.signal.aborted) return;

          const batch = claimed.slice(
            start,
            start + LIST_TRANSLATION_BATCH_SIZE,
          );
          const byItemId = new Map(batch.map((item) => [item.item_id, item]));
          const received = new Set<number>();
          let sawDone = false;

          try {
            await translationAPI.translateItems(
              {
                items: batch.map(({ key: _key, ...item }) => item),
              },
              {
                onItem: (translated) => {
                  if (controller.signal.aborted) return;
                  const input = byItemId.get(translated.item_id);
                  if (!input) return;

                  const titleComplete =
                    !input.title || Boolean(translated.translated_title?.trim());
                  const summaryComplete =
                    !input.summary ||
                    Boolean(translated.translated_summary?.trim());
                  if (!titleComplete || !summaryComplete) return;

                  translationCache.set(input.key, {
                    title: translated.translated_title,
                    summary: translated.translated_summary,
                  });
                  received.add(translated.item_id);
                  bump();
                },
                onDone: () => {
                  sawDone = true;
                },
              },
              controller.signal,
            );
          } catch (requestError) {
            if (controller.signal.aborted) return;

            for (const input of claimed) {
              if (!translationCache.has(input.key)) {
                blockedKeysRef.current.add(input.key);
              }
            }
            failedStatusRef.current = "error";
            setError(
              requestError instanceof Error
                ? requestError.message
                : "translation failed",
            );
            setStatus("error");
            return;
          }

          if (controller.signal.aborted) return;

          const missing = batch.filter(
            (item) => !received.has(item.item_id),
          );
          if (!sawDone || missing.length > 0) {
            for (const input of claimed) {
              if (!translationCache.has(input.key)) {
                blockedKeysRef.current.add(input.key);
              }
            }
            failedStatusRef.current = "incomplete";
            setError("translation incomplete");
            setStatus("incomplete");
            return;
          }
        }
      } finally {
        for (const input of claimed) {
          inFlightKeysRef.current.delete(input.key);
        }
        controllersRef.current.delete(controller);

        if (!controller.signal.aborted && generationRef.current === generation) {
          activeWorkersRef.current = Math.max(
            0,
            activeWorkersRef.current - 1,
          );
          if (activeWorkersRef.current === 0) {
            setStatus(failedStatusRef.current ?? "idle");
            if (!failedStatusRef.current) {
              setError(null);
            }
          }
        }
      }
    })();
  }, [enabled, bump]);

  useEffect(() => {
    startWorker();
  }, [enabled, scopeKey, retryToken, signature, startWorker]);

  const translationsByItemId = new Map<number, ListTranslationText>();
  for (const input of requestItems) {
    const translation = translationCache.get(input.key);
    if (translation) {
      translationsByItemId.set(input.item_id, translation);
    }
  }

  const retry = useCallback(() => {
    setRetryToken((token) => token + 1);
  }, []);

  return {
    translationsByItemId,
    status: enabled ? status : "idle",
    error: enabled ? error : null,
    retry,
  };
}
