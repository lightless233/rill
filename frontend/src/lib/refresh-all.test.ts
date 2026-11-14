import { describe, expect, it } from "vitest";
import {
  INITIAL_REFRESH_ALL_SYNC_STATE,
  markRefreshAllTriggered,
  reconcileRefreshAllState,
} from "./refresh-all";

describe("refresh-all sync state", () => {
  it("tracks running state through delayed completion and triggers invalidation once", () => {
    let state = markRefreshAllTriggered(100);

    let step = reconcileRefreshAllState(state, {
      refreshAllRunning: false,
      dataUpdatedAt: 100,
      isFetching: true,
      isError: false,
    });
    expect(step.isRefreshing).toBe(true);
    expect(step.shouldInvalidate).toBe(false);
    state = step.next;

    step = reconcileRefreshAllState(state, {
      refreshAllRunning: true,
      dataUpdatedAt: 200,
      isFetching: false,
      isError: false,
    });
    expect(step.isRefreshing).toBe(true);
    expect(step.shouldInvalidate).toBe(false);
    state = step.next;

    step = reconcileRefreshAllState(state, {
      refreshAllRunning: false,
      dataUpdatedAt: 1700,
      isFetching: false,
      isError: false,
    });
    expect(step.isRefreshing).toBe(false);
    expect(step.shouldInvalidate).toBe(true);
    state = step.next;

    step = reconcileRefreshAllState(state, {
      refreshAllRunning: false,
      dataUpdatedAt: 1700,
      isFetching: false,
      isError: false,
    });
    expect(step.isRefreshing).toBe(false);
    expect(step.shouldInvalidate).toBe(false);
  });

  it("reconciles fast completion that finishes before the first app poll resolves", () => {
    let state = markRefreshAllTriggered(100);

    // Same-millisecond settled response after observing fetch
    let step = reconcileRefreshAllState(state, {
      refreshAllRunning: false,
      dataUpdatedAt: 100,
      isFetching: true,
      isError: false,
    });
    state = step.next;

    step = reconcileRefreshAllState(state, {
      refreshAllRunning: false,
      dataUpdatedAt: 100,
      isFetching: false,
      isError: false,
    });
    expect(step.shouldInvalidate).toBe(true);
    expect(step.isRefreshing).toBe(false);
  });

  it("adopts already-running background refresh when visiting feeds page", () => {
    const started = reconcileRefreshAllState(INITIAL_REFRESH_ALL_SYNC_STATE, {
      refreshAllRunning: true,
      dataUpdatedAt: 500,
      isFetching: false,
      isError: false,
    });
    expect(started.isRefreshing).toBe(true);
    expect(started.shouldInvalidate).toBe(false);

    const finished = reconcileRefreshAllState(started.next, {
      refreshAllRunning: false,
      dataUpdatedAt: 2000,
      isFetching: false,
      isError: false,
    });
    expect(finished.isRefreshing).toBe(false);
    expect(finished.shouldInvalidate).toBe(true);
  });

  it("clears pending sync state if /api/app stops in an error state", () => {
    const state = markRefreshAllTriggered(100);
    const step = reconcileRefreshAllState(state, {
      refreshAllRunning: true,
      dataUpdatedAt: 100,
      isFetching: false,
      isError: true,
    });
    expect(step.isRefreshing).toBe(false);
    expect(step.shouldInvalidate).toBe(false);
    expect(step.next).toEqual(INITIAL_REFRESH_ALL_SYNC_STATE);
  });

  it("never reports cached refreshAllRunning=true as active while retrying an errored query", () => {
    const step = reconcileRefreshAllState(INITIAL_REFRESH_ALL_SYNC_STATE, {
      refreshAllRunning: true,
      dataUpdatedAt: 200,
      isFetching: true,
      isError: true,
    });
    expect(step.isRefreshing).toBe(false);
    expect(step.shouldInvalidate).toBe(false);
    expect(step.next).toEqual(INITIAL_REFRESH_ALL_SYNC_STATE);
  });
});
