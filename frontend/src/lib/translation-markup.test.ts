import { describe, expect, it } from "vitest";
import { restoreTranslatedMarkup } from "./translation-markup";

describe("restoreTranslatedMarkup", () => {
  it("restores verified links and inline tags cleanly", () => {
    const html = restoreTranslatedMarkup(
      '阅读 <a0>官方 <code>zar</code> 文档</a0> 并使用 <strong>快速压缩</strong>。',
      [{ href: "https://example.com/docs?a=1&b=2" }],
    );

    expect(html).toBe(
      '阅读 <a href="https://example.com/docs?a=1&amp;b=2" target="_blank" rel="noopener noreferrer">官方 <code>zar</code> 文档</a> 并使用 <strong>快速压缩</strong>。',
    );
  });

  it("escapes injected html tags and raw comparisons safely", () => {
    const html = restoreTranslatedMarkup(
      '当 x < 5 时 <script>alert(1)</script> 且 <img src=x onerror=alert(1)>',
      [],
    );

    expect(html).toBe(
      "当 x &lt; 5 时 &lt;script&gt;alert(1)&lt;/script&gt; 且 &lt;img src=x onerror=alert(1)&gt;",
    );
  });

  it("drops out-of-range link placeholders and closes dangling links", () => {
    const html = restoreTranslatedMarkup(
      "<a0>未闭合和 <a3>未知链接</a3>",
      [{ href: "https://safe.test" }],
    );

    expect(html).toBe(
      '<a href="https://safe.test" target="_blank" rel="noopener noreferrer">未闭合和 未知链接</a>',
    );
  });
});
