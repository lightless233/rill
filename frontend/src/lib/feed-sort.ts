import type { Feed, Group } from "@/lib/api";

/**
 * Places feeds with unread items ahead of fully read feeds while preserving
 * relative order within each bucket.
 */
export function sortFeedsUnreadFirst(feeds: Feed[]): Feed[] {
  if (feeds.length <= 1) {
    return feeds;
  }

  const withUnread: Feed[] = [];
  const withoutUnread: Feed[] = [];
  for (const feed of feeds) {
    if ((feed.unread_count || 0) > 0) {
      withUnread.push(feed);
    } else {
      withoutUnread.push(feed);
    }
  }

  return [...withUnread, ...withoutUnread];
}

function applyVisibleGroupOrder(
  allGroups: Group[],
  nextVisibleIds: number[],
): Group[] {
  const byId = new Map(allGroups.map((g) => [g.id, g]));
  const visibleSet = new Set(nextVisibleIds);
  let visibleCursor = 0;

  return allGroups.map((group, idx) => {
    const resolved = visibleSet.has(group.id)
      ? byId.get(nextVisibleIds[visibleCursor++]) ?? group
      : group;
    return {
      ...resolved,
      sort_order: idx + 1,
    };
  });
}

/**
 * Computes a new full `groups` array after dropping `movedGroupId` into gap
 * `dropIndex` (`0..visibleGroupIds.length`) of the visible list. Returns `null`
 * when the drop is a no-op (same slot).
 */
export function reorderGroupsByDropIndex(
  allGroups: Group[],
  visibleGroupIds: number[],
  movedGroupId: number,
  dropIndex: number,
): Group[] | null {
  const fromIndex = visibleGroupIds.indexOf(movedGroupId);
  if (fromIndex === -1) {
    return null;
  }

  const clampedDrop = Math.max(0, Math.min(dropIndex, visibleGroupIds.length));
  if (clampedDrop === fromIndex || clampedDrop === fromIndex + 1) {
    return null;
  }

  const nextVisible = visibleGroupIds.slice();
  const [moved] = nextVisible.splice(fromIndex, 1);
  const insertAt = clampedDrop > fromIndex ? clampedDrop - 1 : clampedDrop;
  nextVisible.splice(insertAt, 0, moved);

  return applyVisibleGroupOrder(allGroups, nextVisible);
}

/**
 * Moves a visible group one slot earlier (`-1`) or later (`1`). Returns `null`
 * at list boundaries or when the group is not in `visibleGroupIds`.
 */
export function moveGroupInVisibleList(
  allGroups: Group[],
  visibleGroupIds: number[],
  groupId: number,
  delta: -1 | 1,
): Group[] | null {
  const fromIndex = visibleGroupIds.indexOf(groupId);
  const toIndex = fromIndex + delta;
  if (fromIndex === -1 || toIndex < 0 || toIndex >= visibleGroupIds.length) {
    return null;
  }

  const nextVisible = visibleGroupIds.slice();
  const [moved] = nextVisible.splice(fromIndex, 1);
  nextVisible.splice(toIndex, 0, moved);

  return applyVisibleGroupOrder(allGroups, nextVisible);
}
