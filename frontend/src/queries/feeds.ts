import { useMemo, useCallback } from "react";
import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query";
import { feedAPI, type Feed } from "@/lib/api";
import { queryKeys } from "./keys";

export const feedQueries = {
  list: () =>
    queryOptions({
      queryKey: queryKeys.feeds.list(),
      queryFn: async () => {
        const res = await feedAPI.list();
        return res.data;
      },
    }),
};

/**
 * Refetches feeds without reusing a pending request that may predate the
 * notification. fetchQuery also creates the query when the first sidebar
 * observer has not mounted yet, which invalidateQueries alone cannot do.
 */
export async function refreshFeedsQuery(qc: QueryClient): Promise<void> {
  const query = feedQueries.list();
  void qc.invalidateQueries({
    queryKey: query.queryKey,
    refetchType: "none",
  });
  await qc.cancelQueries({ queryKey: query.queryKey });
  await qc.fetchQuery({ ...query, staleTime: 0 });
}

export function useFeeds() {
  return useQuery(feedQueries.list());
}

export function useFeedLookup() {
  const { data, isLoading, isError, isSuccess, error, refetch } = useFeeds();
  const feeds = useMemo(() => data ?? [], [data]);

  const feedMap = useMemo(() => new Map(feeds.map((f) => [f.id, f])), [feeds]);

  const getFeedById = useCallback(
    (feedId: number) => feedMap.get(feedId),
    [feedMap],
  );

  const getFeedsByGroup = useCallback(
    (groupId: number) => feeds.filter((f) => f.group_id === groupId),
    [feeds],
  );

  return {
    feeds,
    getFeedById,
    getFeedsByGroup,
    isLoading,
    isError,
    isSuccess,
    error,
    refetch,
  };
}

export function useUnreadCounts() {
  const { data: feeds, isError, error } = useFeeds();

  const getUnreadCount = useCallback(
    (feedId: number) =>
      feeds ? (feeds.find((f) => f.id === feedId)?.unread_count ?? 0) : undefined,
    [feeds],
  );

  const getGroupUnreadCount = useCallback(
    (groupId: number) =>
      feeds
        ? feeds
            .filter((f) => f.group_id === groupId)
            .reduce((sum, f) => sum + (f.unread_count ?? 0), 0)
        : undefined,
    [feeds],
  );

  const getTotalUnreadCount = useCallback(
    () =>
      feeds
        ? feeds.reduce((sum, f) => sum + (f.unread_count ?? 0), 0)
        : undefined,
    [feeds],
  );

  return {
    hasData: feeds !== undefined,
    isError,
    error,
    getUnreadCount,
    getGroupUnreadCount,
    getTotalUnreadCount,
  };
}

export function applyFeedCreatedCache(qc: QueryClient, feed: Feed) {
  qc.setQueryData(queryKeys.feeds.list(), (old: Feed[] | undefined) =>
    old
      ? old.some((existing) => existing.id === feed.id)
        ? old
        : [...old, feed]
      : [feed],
  );
  void qc.invalidateQueries({ queryKey: queryKeys.feeds.all });
  void qc.invalidateQueries({ queryKey: queryKeys.items.all });
}

export function useCreateFeed() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (req: Parameters<typeof feedAPI.create>[0]) => {
      const res = await feedAPI.create(req);
      return res.data!;
    },
    onSuccess: (feed) => {
      applyFeedCreatedCache(qc, feed);
    },
  });
}

export function useUpdateFeed() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({
      id,
      ...data
    }: Parameters<typeof feedAPI.update>[1] & { id: number }) => {
      const res = await feedAPI.update(id, data);
      return res.data!;
    },
    onSuccess: (updated) => {
      qc.setQueryData(queryKeys.feeds.list(), (old: Feed[] | undefined) =>
        old?.map((f) => (f.id === updated.id ? updated : f)),
      );
    },
  });
}

export function useDeleteFeed() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: number) => {
      await feedAPI.delete(id);
      return id;
    },
    onSuccess: (id) => {
      qc.setQueryData(queryKeys.feeds.list(), (old: Feed[] | undefined) =>
        old?.filter((f) => f.id !== id),
      );
      qc.invalidateQueries({ queryKey: queryKeys.items.all });
    },
  });
}

export function useRefreshFeeds() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => feedAPI.refresh(),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.app.info() });
      qc.invalidateQueries({ queryKey: queryKeys.feeds.all });
      qc.invalidateQueries({ queryKey: queryKeys.items.all });
    },
  });
}

export function useMoveFeedsToGroup() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({
      fromGroupId,
      toGroupId,
    }: {
      fromGroupId: number;
      toGroupId: number;
    }) => {
      const feeds = qc.getQueryData<Feed[]>(queryKeys.feeds.list()) ?? [];
      const toMove = feeds.filter((f) => f.group_id === fromGroupId);
      await Promise.all(
        toMove.map((f) => feedAPI.update(f.id, { group_id: toGroupId })),
      );
      return { fromGroupId, toGroupId };
    },
    onSuccess: ({ fromGroupId, toGroupId }) => {
      qc.setQueryData(queryKeys.feeds.list(), (old: Feed[] | undefined) =>
        old?.map((f) =>
          f.group_id === fromGroupId ? { ...f, group_id: toGroupId } : f,
        ),
      );
    },
  });
}
