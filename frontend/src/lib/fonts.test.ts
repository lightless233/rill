import { describe, expect, it } from "vitest";
import {
  LATIN_FONT_OPTIONS,
  CJK_FONT_OPTIONS,
  MONO_FONT_OPTIONS,
  resolveArticleBodyFontFamily,
  resolveArticleMonoFontFamily,
} from "./fonts";

describe("article font catalog & stacks", () => {
  it("provides both sans-serif and serif options for Latin and CJK fonts", () => {
    expect(LATIN_FONT_OPTIONS.some((f) => f.category === "sans")).toBe(true);
    expect(LATIN_FONT_OPTIONS.some((f) => f.category === "serif")).toBe(true);
    expect(CJK_FONT_OPTIONS.some((f) => f.category === "sans")).toBe(true);
    expect(CJK_FONT_OPTIONS.some((f) => f.category === "serif")).toBe(true);
    expect(MONO_FONT_OPTIONS.length).toBeGreaterThanOrEqual(3);
  });

  it("places Latin font ahead of CJK font so Latin and CJK selections compose cleanly", () => {
    const stack = resolveArticleBodyFontFamily("newsreader", "noto-serif-sc");
    const newsreaderIndex = stack.indexOf('"Newsreader Variable"');
    const cjkIndex = stack.indexOf('"Noto Serif SC Variable"');

    expect(newsreaderIndex).toBe(0);
    expect(cjkIndex).toBeGreaterThan(newsreaderIndex);
  });

  it("keeps system Latin options free of system-ui/-apple-system so CJK selection still applies", () => {
    const systemSans = LATIN_FONT_OPTIONS.find((f) => f.value === "system-sans");
    const systemSerif = LATIN_FONT_OPTIONS.find((f) => f.value === "system-serif");

    expect(systemSans?.family).not.toContain("system-ui");
    expect(systemSans?.family).not.toContain("-apple-system");
    expect(systemSerif?.family).not.toContain("system-ui");
    expect(systemSerif?.family).not.toContain("-apple-system");
  });

  it("puts monospace first, preserves CJK fallback, and ends with monospace generic", () => {
    const monoStack = resolveArticleMonoFontFamily("jetbrains-mono", "lxgw-wenkai");

    expect(monoStack.startsWith('"JetBrains Mono Variable",')).toBe(true);
    expect(monoStack).toContain('"LXGW WenKai Screen"');
    expect(monoStack.endsWith(", monospace")).toBe(true);
    expect(monoStack).not.toContain(", serif, monospace");
  });
});
