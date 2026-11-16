export const articleFilters = ["all", "unread", "starred"] as const;

export type ArticleFilter = (typeof articleFilters)[number];

export const defaultArticleFilter: ArticleFilter = "unread";

export function isArticleFilter(value: string): value is ArticleFilter {
  return articleFilters.includes(value as ArticleFilter);
}

// Selecting a feed with no unread articles from the unread view would land on
// an empty tab; fall back to all only for that specific navigation.
export function filterAfterFeedSelection(
  currentFilter: ArticleFilter,
  unreadCount: number,
): ArticleFilter {
  return currentFilter === "unread" && unreadCount <= 0 ? "all" : currentFilter;
}
