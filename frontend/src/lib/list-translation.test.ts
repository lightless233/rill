import { describe, expect, it, vi } from "vitest";

vi.mock("dompurify", () => ({
  default: {
    sanitize: (html: string) => html.replace(/<[^>]*>/g, ""),
  },
}));

import type { Item } from "@/lib/api";
import {
  buildListTranslationInput,
  effectiveListTranslationMode,
  isEligibleForTranslation,
  listTranslationSignature,
} from "./list-translation";
import { extractSummary, truncateTextAtBoundary } from "./utils";

function item(overrides: Partial<Item> = {}): Item {
  return {
    id: 1,
    feed_id: 2,
    guid: "guid-1",
    title: "An English title",
    link: "https://example.com/post",
    content: "<p>An English summary with enough words to translate.</p>",
    pub_date: 0,
    unread: true,
    created_at: 0,
    bookmark_id: null,
    ...overrides,
  };
}

describe("list translation eligibility", () => {
  it("accepts Latin-only text and rejects Chinese or mixed text", () => {
    expect(isEligibleForTranslation("Hello world")).toBe(true);
    expect(isEligibleForTranslation("你好，世界")).toBe(false);
    expect(isEligibleForTranslation("OpenAI 发布新模型")).toBe(false);
    expect(isEligibleForTranslation("")).toBe(false);
  });

  it("derives the default mode from key availability without losing an explicit choice", () => {
    expect(effectiveListTranslationMode(null, true)).toBe("bilingual");
    expect(effectiveListTranslationMode(null, false)).toBe("original");
    expect(effectiveListTranslationMode("original", true)).toBe("original");
    expect(effectiveListTranslationMode("zh", false)).toBe("original");
  });

  it("leaves Chinese fields out of the request while translating eligible fields", () => {
    const input = buildListTranslationInput(
      item({ title: "中文标题", content: "<p>An English summary.</p>" }),
    );
    expect(input?.title).toBeUndefined();
    expect(input?.summary).toContain("An English summary");
  });

  it("keeps a stable content key for identical content", () => {
    const a = buildListTranslationInput(item());
    const b = buildListTranslationInput(item());
    expect(a?.key).toBe(b?.key);
    expect(listTranslationSignature(a ? [a] : [])).toBe(a?.key);
  });
});

describe("translation source truncation", () => {
  it("cuts at a sentence boundary when possible", () => {
    expect(
      truncateTextAtBoundary(
        "First sentence. Second sentence is longer than the limit.",
        20,
      ),
    ).toBe("First sentence.");
  });

  it("falls back to a word boundary instead of cutting mid-word", () => {
    expect(
      truncateTextAtBoundary("alpha beta gamma delta", 12),
    ).toBe("alpha beta");
  });

  it("keeps the existing display excerpt behavior", () => {
    expect(extractSummary("<p>One two three four</p>", 8)).toBe("One two…");
  });
});
