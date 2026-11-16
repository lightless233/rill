import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { APIError, eventsAPI } from "@/lib/api";
import { isAbortError } from "@/lib/query-status";
import { refreshFeedsQuery } from "@/queries/feeds";

const INITIAL_RETRY_MS = 1_000;
const MAX_RETRY_MS = 30_000;

function retryDelay(attempt: number): number {
  return Math.min(MAX_RETRY_MS, INITIAL_RETRY_MS * 2 ** Math.min(attempt, 5));
}

/**
 * Keeps the authenticated feed-change SSE stream alive while the app shell is
 * mounted. Events are signals only; the feeds query remains the authority for
 * unread counts.
 */
export function useFeedEvents() {
  const queryClient = useQueryClient();

  useEffect(() => {
    let disposed = false;
    let attempt = 0;
    let controller: AbortController | null = null;
    let retryTimer: number | undefined;

    const invalidateFeeds = () => {
      void refreshFeedsQuery(queryClient).catch(() => {
        // A failed refetch is recovered by the next event or reconnect; the
        // global 401 handler owns session failures.
      });
    };

    const scheduleReconnect = () => {
      if (disposed) return;
      const delay = retryDelay(attempt);
      attempt += 1;
      retryTimer = window.setTimeout(() => {
        retryTimer = undefined;
        void connect();
      }, delay);
    };

    const connect = async () => {
      if (disposed) return;
      controller = new AbortController();

      try {
        await eventsAPI.stream((eventName) => {
          if (eventName === "ready") {
            attempt = 0;
            invalidateFeeds();
          } else if (eventName === "feeds_changed") {
            invalidateFeeds();
          }
        }, controller.signal);

        if (!disposed) {
          scheduleReconnect();
        }
      } catch (error) {
        if (disposed || isAbortError(error)) return;
        // The global API client has already redirected on 401. Retrying a
        // known unauthorized stream would only loop until the page unloads.
        if (error instanceof APIError && error.status === 401) {
          return;
        }
        scheduleReconnect();
      }
    };

    void connect();

    return () => {
      disposed = true;
      controller?.abort();
      if (retryTimer !== undefined) {
        window.clearTimeout(retryTimer);
      }
    };
  }, [queryClient]);
}
