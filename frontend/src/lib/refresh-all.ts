export interface RefreshAllSyncState {
  pendingSinceUpdatedAt: number | null;
  sawFetching: boolean;
  wasRunning: boolean;
}

export interface AppInfoSyncSnapshot {
  refreshAllRunning?: boolean;
  dataUpdatedAt: number;
  isFetching: boolean;
  isError: boolean;
}

export interface RefreshAllSyncResult {
  next: RefreshAllSyncState;
  shouldInvalidate: boolean;
  isRefreshing: boolean;
}

export const INITIAL_REFRESH_ALL_SYNC_STATE: RefreshAllSyncState = {
  pendingSinceUpdatedAt: null,
  sawFetching: false,
  wasRunning: false,
};

/** Marks a user-initiated refresh-all trigger so fast completions still reconcile. */
export function markRefreshAllTriggered(
  currentDataUpdatedAt: number,
): RefreshAllSyncState {
  return {
    pendingSinceUpdatedAt: currentDataUpdatedAt,
    sawFetching: false,
    wasRunning: false,
  };
}

/**
 * Reconciles /api/app refresh_all_running status against local trigger state.
 * Emits shouldInvalidate once a running cycle finishes or a post-trigger fetch settles false.
 */
export function reconcileRefreshAllState(
  prev: RefreshAllSyncState,
  snapshot: AppInfoSyncSnapshot,
): RefreshAllSyncResult {
  if (snapshot.isError) {
    return {
      next: INITIAL_REFRESH_ALL_SYNC_STATE,
      shouldInvalidate: false,
      isRefreshing: false,
    };
  }

  if (snapshot.refreshAllRunning === true) {
    return {
      next: {
        pendingSinceUpdatedAt: null,
        sawFetching: false,
        wasRunning: true,
      },
      shouldInvalidate: false,
      isRefreshing: true,
    };
  }

  if (prev.wasRunning && snapshot.refreshAllRunning === false) {
    return {
      next: INITIAL_REFRESH_ALL_SYNC_STATE,
      shouldInvalidate: true,
      isRefreshing: false,
    };
  }

  if (prev.pendingSinceUpdatedAt !== null) {
    const sawFetching = prev.sawFetching || snapshot.isFetching;
    const hasSettledAfterTrigger =
      snapshot.refreshAllRunning === false &&
      (snapshot.dataUpdatedAt > prev.pendingSinceUpdatedAt ||
        (sawFetching && !snapshot.isFetching));

    if (hasSettledAfterTrigger) {
      return {
        next: INITIAL_REFRESH_ALL_SYNC_STATE,
        shouldInvalidate: true,
        isRefreshing: false,
      };
    }

    return {
      next: {
        pendingSinceUpdatedAt: prev.pendingSinceUpdatedAt,
        sawFetching,
        wasRunning: false,
      },
      shouldInvalidate: false,
      isRefreshing: true,
    };
  }

  return {
    next: prev,
    shouldInvalidate: false,
    isRefreshing: prev.wasRunning,
  };
}
