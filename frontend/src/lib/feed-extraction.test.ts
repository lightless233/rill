import { describe, expect, it } from "vitest";
import { feedSelectorUpdate } from "./feed-extraction";

describe("feedSelectorUpdate", () => {
  it("omits an untouched selector, including missing and whitespace-only values", () => {
    expect(feedSelectorUpdate("", "")).toBeUndefined();
    expect(feedSelectorUpdate(undefined, "   ")).toBeUndefined();
    expect(feedSelectorUpdate(" article ", "article")).toBeUndefined();
  });

  it("returns the changed trimmed selector", () => {
    expect(feedSelectorUpdate("", " article .body ")).toBe("article .body");
    expect(feedSelectorUpdate("article", "main")).toBe("main");
  });

  it("returns an explicit empty string for a deliberate clear", () => {
    expect(feedSelectorUpdate("article", "")).toBe("");
    expect(feedSelectorUpdate("article", "   ")).toBe("");
  });
});
