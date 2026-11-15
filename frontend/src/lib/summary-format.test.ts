import { describe, expect, it } from "vitest";
import { renderSummaryMarkdownHtml } from "./summary-format";

describe("renderSummaryMarkdownHtml", () => {
  it("renders summary paragraphs, headings, lists, bold, and inline code", () => {
    const html = renderSummaryMarkdownHtml(
      [
        "本文总结了边缘缓存架构的核心收益。",
        "",
        "### 核心要点",
        "- 利用 `stale-while-revalidate` 与 **区域节点** 降低回源压力",
        "- 减少 40% P95 延迟",
        "",
        "1. 第一步：开启缓存头",
        "2. 第二步：配置回源屏蔽",
      ].join("\n"),
    );

    expect(html).toContain("<p>本文总结了边缘缓存架构的核心收益。</p>");
    expect(html).toContain("<h4>核心要点</h4>");
    expect(html).toContain(
      "<ul><li>利用 <code>stale-while-revalidate</code> 与 <strong>区域节点</strong> 降低回源压力</li><li>减少 40% P95 延迟</li></ul>",
    );
    expect(html).toContain(
      "<ol><li>第一步：开启缓存头</li><li>第二步：配置回源屏蔽</li></ol>",
    );
  });

  it("escapes raw HTML instead of passing tags through", () => {
    const html = renderSummaryMarkdownHtml(
      '- Payload <script>alert(1)</script> inside `snake_case_var`',
    );
    expect(html).toBe(
      "<ul><li>Payload &lt;script&gt;alert(1)&lt;/script&gt; inside <code>snake_case_var</code></li></ul>",
    );
  });

  it("returns empty string for blank input", () => {
    expect(renderSummaryMarkdownHtml("   \n\t  ")).toBe("");
  });
});
