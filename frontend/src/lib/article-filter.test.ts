import { describe, expect, it } from "vitest";
import {
  articleFilters,
  defaultArticleFilter,
  filterAfterFeedSelection,
  isArticleFilter,
} from "./article-filter";

describe("article filter set", () => {
  it("accepts every declared filter", () => {
    for (const filter of articleFilters) {
      expect(isArticleFilter(filter)).toBe(true);
    }
  });

  it("rejects anything else, including near-misses", () => {
    expect(isArticleFilter("")).toBe(false);
    expect(isArticleFilter("Unread")).toBe(false);
    expect(isArticleFilter("read")).toBe(false);
    expect(isArticleFilter("favorites")).toBe(false);
  });

  it("defaults to the unread view", () => {
    expect(defaultArticleFilter).toBe("unread");
    expect(articleFilters).toContain(defaultArticleFilter);
  });

  it("falls back to all when selecting a feed with no unread articles from unread", () => {
    expect(filterAfterFeedSelection("unread", 0)).toBe("all");
    expect(filterAfterFeedSelection("unread", 3)).toBe("unread");
    expect(filterAfterFeedSelection("all", 0)).toBe("all");
    expect(filterAfterFeedSelection("starred", 0)).toBe("starred");
  });
});
