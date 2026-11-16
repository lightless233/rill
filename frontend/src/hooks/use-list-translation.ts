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
  const blockedKeysRef = useRef<Set<string>>(new Set());

  useEffect(() => {
    if (!enabled) {
      return;
    }

    const controller = new AbortController();
    let cancelled = false;
    const inputs = requestItemsRef.current;
    const pending = inputs.filter(
      (item) =>
        !translationCache.has(item.key) &&
        !blockedKeysRef.current.has(item.key),
    );

    void (async () => {
      if (cancelled) return;
      if (pending.length === 0) {
        setStatus(blockedKeysRef.current.size > 0 ? "error" : "idle");
        return;
      }

      setStatus("translating");
      setError(null);
      for (
        let start = 0;
        start < pending.length;
        start += LIST_TRANSLATION_BATCH_SIZE
      ) {
        if (cancelled || controller.signal.aborted) return;

        const batch = pending.slice(
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
                if (cancelled || controller.signal.aborted) return;
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
          if (cancelled || controller.signal.aborted) return;

          for (const input of batch) {
            blockedKeysRef.current.add(input.key);
          }
          setError(
            requestError instanceof Error
              ? requestError.message
              : "translation failed",
          );
          setStatus("error");
          return;
        }

        if (cancelled || controller.signal.aborted) return;

        const missing = batch.filter(
          (item) => !received.has(item.item_id),
        );
        if (!sawDone || missing.length > 0) {
          for (const input of missing) {
            blockedKeysRef.current.add(input.key);
          }
          setError("translation incomplete");
          setStatus("incomplete");
          return;
        }
      }

      if (cancelled) return;
      setStatus(blockedKeysRef.current.size > 0 ? "error" : "idle");
    })();

    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [enabled, signature, scopeKey, retryToken, bump]);

  const translationsByItemId = new Map<number, ListTranslationText>();
  for (const input of requestItems) {
    const translation = translationCache.get(input.key);
    if (translation) {
      translationsByItemId.set(input.item_id, translation);
    }
  }

  const retry = useCallback(() => {
    blockedKeysRef.current.clear();
    setError(null);
    setStatus("idle");
    setRetryToken((token) => token + 1);
  }, []);

  return {
    translationsByItemId,
    status: enabled ? status : "idle",
    error: enabled ? error : null,
    retry,
  };
}
