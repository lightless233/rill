import type { Feed, Group } from "@/lib/api";
import type { ArticleFilter } from "@/lib/article-filter";
import { sortFeedsUnreadFirst } from "@/lib/feed-sort";
import { shouldHideDefaultGroup } from "@/lib/group-visibility";

export type UnreadNavigationTarget =
  | { kind: "feed"; feedId: number }
  | { kind: "group"; groupId: number }
  | { kind: "all" };

export interface ResolveNextUnreadTargetInput {
  groups: Group[];
  /** Feeds snapshot before marking items read, preserving visual sidebar slots. */
  beforeFeeds: Feed[];
  /** Latest feeds list after the mark-read action settles. */
  afterFeeds: Feed[];
  isFeedsSuccess: boolean;
  isFeedsError: boolean;
  selectedFeedId: number | null;
  selectedGroupId: number | null;
  articleFilter: ArticleFilter;
}

export function getScopeUnreadCount(
  feeds: Feed[],
  selectedFeedId: number | null,
  selectedGroupId: number | null,
): number {
  if (selectedFeedId !== null) {
    return feeds.find((f) => f.id === selectedFeedId)?.unread_count ?? 0;
  }
  if (selectedGroupId !== null) {
    return feeds.reduce(
      (sum, f) =>
        f.group_id === selectedGroupId ? sum + (f.unread_count || 0) : sum,
      0,
    );
  }
  return feeds.reduce((sum, f) => sum + (f.unread_count || 0), 0);
}

function getVisibleGroups(
  groups: Group[],
  feeds: Feed[],
  isFeedsSuccess: boolean,
  isFeedsError: boolean,
): Group[] {
  const counts = new Map<number, number>();
  for (const feed of feeds) {
    counts.set(feed.group_id, (counts.get(feed.group_id) ?? 0) + 1);
  }

  return groups.filter(
    (group) =>
      !shouldHideDefaultGroup({
        groupId: group.id,
        unfilteredGroupFeedCount: counts.get(group.id) ?? 0,
        isFeedsSuccess,
        isFeedsError,
      }),
  );
}

function buildSidebarOrderedFeedIds(
  groups: Group[],
  beforeFeeds: Feed[],
  afterFeeds: Feed[],
  isFeedsSuccess: boolean,
  isFeedsError: boolean,
): number[] {
  const visibleGroups = getVisibleGroups(
    groups,
    beforeFeeds,
    isFeedsSuccess,
    isFeedsError,
  );
  const byGroup = new Map<number, Feed[]>();
  const ungrouped: Feed[] = [];
  for (const feed of beforeFeeds) {
    if (feed.group_id === 0) {
      ungrouped.push(feed);
      continue;
    }
    const list = byGroup.get(feed.group_id);
    if (list) {
      list.push(feed);
    } else {
      byGroup.set(feed.group_id, [feed]);
    }
  }

  const orderedIds: number[] = [];
  const seenIds = new Set<number>();

  for (const group of visibleGroups) {
    for (const feed of sortFeedsUnreadFirst(byGroup.get(group.id) ?? [])) {
      orderedIds.push(feed.id);
      seenIds.add(feed.id);
    }
  }

  for (const feed of sortFeedsUnreadFirst(ungrouped)) {
    orderedIds.push(feed.id);
    seenIds.add(feed.id);
  }

  // Include feeds from visible/ungrouped snapshots or newly fetched feeds if absent from beforeFeeds.
  for (const feed of afterFeeds) {
    if (!seenIds.has(feed.id)) {
      orderedIds.push(feed.id);
      seenIds.add(feed.id);
    }
  }

  return orderedIds;
}

function pickWrappedCandidate<T>(
  orderedItems: T[],
  isCurrent: (item: T) => boolean,
  hasUnread: (item: T) => boolean,
): T | null {
  if (orderedItems.length === 0) return null;
  const currentIndex = orderedItems.findIndex(isCurrent);
  const start = currentIndex === -1 ? 0 : currentIndex + 1;

  for (let offset = 0; offset < orderedItems.length; offset++) {
    const candidate = orderedItems[(start + offset) % orderedItems.length];
    if (!isCurrent(candidate) && hasUnread(candidate)) {
      return candidate;
    }
  }

  return null;
}

/**
 * Resolves the next destination after clearing the current feed or group's unread queue.
 * Returns `null` when auto-advance does not apply (non-unread tabs or global scope).
 */
export function resolveNextUnreadTarget({
  groups,
  beforeFeeds,
  afterFeeds,
  isFeedsSuccess,
  isFeedsError,
  selectedFeedId,
  selectedGroupId,
  articleFilter,
}: ResolveNextUnreadTargetInput): UnreadNavigationTarget | null {
  if (
    articleFilter !== "unread" ||
    (selectedFeedId === null && selectedGroupId === null)
  ) {
    return null;
  }

  if (selectedGroupId !== null) {
    const visibleGroups = getVisibleGroups(
      groups,
      beforeFeeds,
      isFeedsSuccess,
      isFeedsError,
    );
    const groupUnread = new Map<number, number>();
    for (const feed of afterFeeds) {
      if ((feed.unread_count || 0) > 0) {
        groupUnread.set(
          feed.group_id,
          (groupUnread.get(feed.group_id) ?? 0) + feed.unread_count,
        );
      }
    }

    const nextGroup = pickWrappedCandidate(
      visibleGroups,
      (g) => g.id === selectedGroupId,
      (g) => (groupUnread.get(g.id) ?? 0) > 0,
    );
    return nextGroup
      ? { kind: "group", groupId: nextGroup.id }
      : { kind: "all" };
  }

  const orderedFeedIds = buildSidebarOrderedFeedIds(
    groups,
    beforeFeeds,
    afterFeeds,
    isFeedsSuccess,
    isFeedsError,
  );
  const feedUnread = new Map<number, number>(
    afterFeeds.map((f) => [f.id, f.unread_count || 0]),
  );

  const nextFeedId = pickWrappedCandidate(
    orderedFeedIds,
    (id) => id === selectedFeedId,
    (id) => (feedUnread.get(id) ?? 0) > 0,
  );
  return nextFeedId !== null
    ? { kind: "feed", feedId: nextFeedId }
    : { kind: "all" };
}
