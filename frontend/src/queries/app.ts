import { queryOptions, useQuery } from "@tanstack/react-query";
import { appAPI } from "@/lib/api";
import { queryKeys } from "./keys";

export const appQueries = {
  info: () =>
    queryOptions({
      queryKey: queryKeys.app.info(),
      queryFn: async () => {
        const res = await appAPI.getInfo();
        return res.data!;
      },
    }),
};

export const REFRESH_ALL_POLL_INTERVAL_MS = 1500;

export function useAppInfo(options?: {
  pollWhileRefreshAll?: boolean;
  forcePoll?: boolean;
}) {
  return useQuery({
    ...appQueries.info(),
    ...(options?.pollWhileRefreshAll ? { staleTime: 0 } : {}),
    refetchInterval: options?.pollWhileRefreshAll
      ? (query) => {
          if (options?.forcePoll) {
            return REFRESH_ALL_POLL_INTERVAL_MS;
          }
          if (query.state.status === "error") {
            return false;
          }
          return query.state.data?.refresh_all_running
            ? REFRESH_ALL_POLL_INTERVAL_MS
            : false;
        }
      : false,
  });
}
