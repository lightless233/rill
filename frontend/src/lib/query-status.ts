export function isAbortError(error: unknown): boolean {
  return error instanceof Error && error.name === "AbortError";
}

export interface AutoFetchEmptyPageInput {
  isLoading: boolean;
  isLoadingMore: boolean;
  isError: boolean;
  isFetchNextPageError: boolean;
  itemCount: number;
  hasMore: boolean;
}

/**
 * Only auto-fetch additional pages on an empty visible slice while queries are healthy;
 * retrying on error inside useEffect would spin an infinite failing request loop.
 */
export function shouldAutoFetchEmptyPage(input: AutoFetchEmptyPageInput): boolean {
  if (
    input.isLoading ||
    input.isLoadingMore ||
    input.isError ||
    input.isFetchNextPageError
  ) {
    return false;
  }
  return input.itemCount === 0 && input.hasMore;
}
