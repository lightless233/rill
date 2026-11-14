import { describe, expect, it } from "vitest";
import { isAbortError, shouldAutoFetchEmptyPage } from "./query-status";

describe("query status helpers", () => {
  it("recognizes AbortError instances cleanly", () => {
    const abortErr = new DOMException("Aborted", "AbortError");
    expect(isAbortError(abortErr)).toBe(true);
    expect(isAbortError(new Error("Network failure"))).toBe(false);
    expect(isAbortError("AbortError")).toBe(false);
  });

  it("blocks auto-fetching empty pages when initial or pagination query failed", () => {
    expect(
      shouldAutoFetchEmptyPage({
        isLoading: false,
        isLoadingMore: false,
        isError: false,
        isFetchNextPageError: false,
        itemCount: 0,
        hasMore: true,
      }),
    ).toBe(true);

    expect(
      shouldAutoFetchEmptyPage({
        isLoading: false,
        isLoadingMore: false,
        isError: true,
        isFetchNextPageError: false,
        itemCount: 0,
        hasMore: true,
      }),
    ).toBe(false);

    expect(
      shouldAutoFetchEmptyPage({
        isLoading: false,
        isLoadingMore: false,
        isError: false,
        isFetchNextPageError: true,
        itemCount: 0,
        hasMore: true,
      }),
    ).toBe(false);
  });
});
