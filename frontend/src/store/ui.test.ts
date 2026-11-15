import { describe, expect, it } from "vitest";
import {
  ARTICLE_DRAWER_EDGE_MARGIN,
  ARTICLE_DRAWER_MIN_WIDTH,
  clampArticleDrawerWidth,
} from "./ui";

describe("clampArticleDrawerWidth", () => {
  it("clamps below min width to ARTICLE_DRAWER_MIN_WIDTH", () => {
    expect(clampArticleDrawerWidth(320, 1440)).toBe(ARTICLE_DRAWER_MIN_WIDTH);
  });

  it("clamps oversized widths to leave the outer edge margin", () => {
    expect(clampArticleDrawerWidth(1600, 1440)).toBe(
      1440 - ARTICLE_DRAWER_EDGE_MARGIN,
    );
  });

  it("falls back safely to min width on narrow viewports", () => {
    expect(clampArticleDrawerWidth(900, 400)).toBe(ARTICLE_DRAWER_MIN_WIDTH);
  });

  it("rounds fractional widths inside bounds", () => {
    expect(clampArticleDrawerWidth(964.6, 1440)).toBe(965);
  });
});
