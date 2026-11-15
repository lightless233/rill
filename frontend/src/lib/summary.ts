import DOMPurify from "dompurify";
import { renderSummaryMarkdownHtml } from "./summary-format";

const purify = DOMPurify(window);

/**
 * Extracts clean plain text from HTML article content for AI summarization,
 * dropping non-content tags and keeping block boundaries readable.
 */
export function extractArticleSummaryText(rawHtml: string): string {
  if (!rawHtml.trim()) return "";

  const parser = new DOMParser();
  const doc = parser.parseFromString(rawHtml, "text/html");
  for (const el of doc.body.querySelectorAll(
    "script, style, noscript, iframe, svg",
  )) {
    el.remove();
  }

  const blockTags = new Set([
    "h1",
    "h2",
    "h3",
    "h4",
    "h5",
    "h6",
    "p",
    "li",
    "blockquote",
    "pre",
    "tr",
  ]);
  const lines: string[] = [];
  let buffer = "";

  const flush = () => {
    const text = buffer.replace(/\s+/g, " ").trim();
    if (text) {
      lines.push(text);
    }
    buffer = "";
  };

  const walk = (node: Node) => {
    if (node.nodeType === Node.TEXT_NODE) {
      buffer += node.textContent ?? "";
      return;
    }
    if (node.nodeType !== Node.ELEMENT_NODE) return;

    const el = node as HTMLElement;
    const tag = el.tagName.toLowerCase();
    if (tag === "br") {
      buffer += " ";
      return;
    }

    const isBlock = blockTags.has(tag);
    if (isBlock) flush();

    if (tag === "pre") {
      const codeText = el.textContent?.trim() ?? "";
      if (codeText) {
        lines.push(codeText);
      }
      return;
    }

    for (const child of el.childNodes) {
      walk(child);
    }
    if (isBlock) {
      flush();
    } else if (tag === "td" || tag === "th") {
      buffer += " | ";
    }
  };

  walk(doc.body);
  flush();
  return lines.join("\n\n");
}

export function renderSafeSummaryHtml(markdown: string): string {
  const html = renderSummaryMarkdownHtml(markdown);
  if (!html) return "";
  return purify.sanitize(html, {
    ALLOWED_TAGS: ["h4", "p", "ul", "ol", "li", "strong", "em", "code", "br"],
    ALLOWED_ATTR: [],
  });
}
