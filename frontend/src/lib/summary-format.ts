import { escapeHtmlText } from "./translation-markup";

function formatInlineSegment(raw: string): string {
  const escaped = escapeHtmlText(raw);
  return escaped
    .replace(/\*\*([^*\n]+)\*\*/g, "<strong>$1</strong>")
    .replace(/\*([^*\n]+)\*/g, "<em>$1</em>");
}

function renderInlineMarkdown(text: string): string {
  let out = "";
  let lastIndex = 0;
  const codeRe = /`([^`\n]+)`/g;

  for (const match of text.matchAll(codeRe)) {
    const index = match.index ?? 0;
    if (index > lastIndex) {
      out += formatInlineSegment(text.slice(lastIndex, index));
    }
    out += `<code>${escapeHtmlText(match[1])}</code>`;
    lastIndex = index + match[0].length;
  }

  if (lastIndex < text.length) {
    out += formatInlineSegment(text.slice(lastIndex));
  }
  return out;
}

/**
 * Renders concise AI summary Markdown (headings, paragraphs, bullet/numbered
 * lists, bold, italic, and inline code) to strictly escaped HTML.
 */
export function renderSummaryMarkdownHtml(markdown: string): string {
  const normalized = markdown.replace(/\r\n?/g, "\n").trim();
  if (!normalized) return "";

  const lines = normalized.split("\n");
  const blocks: string[] = [];
  let paragraphLines: string[] = [];
  let listType: "ul" | "ol" | null = null;
  let listItems: string[] = [];

  const flushParagraph = () => {
    if (paragraphLines.length === 0) return;
    const joined = paragraphLines.map((line) => renderInlineMarkdown(line)).join("<br />");
    blocks.push(`<p>${joined}</p>`);
    paragraphLines = [];
  };

  const flushList = () => {
    if (!listType || listItems.length === 0) return;
    const itemsHtml = listItems
      .map((item) => `<li>${renderInlineMarkdown(item)}</li>`)
      .join("");
    blocks.push(`<${listType}>${itemsHtml}</${listType}>`);
    listType = null;
    listItems = [];
  };

  for (const rawLine of lines) {
    const line = rawLine.trim();
    if (!line) {
      flushParagraph();
      flushList();
      continue;
    }

    const headingMatch = line.match(/^#{1,4}\s+(.+)$/);
    if (headingMatch) {
      flushParagraph();
      flushList();
      blocks.push(`<h4>${renderInlineMarkdown(headingMatch[1].trim())}</h4>`);
      continue;
    }

    const ulMatch = line.match(/^[-*•]\s+(.+)$/);
    if (ulMatch) {
      flushParagraph();
      if (listType !== "ul") {
        flushList();
        listType = "ul";
      }
      listItems.push(ulMatch[1].trim());
      continue;
    }

    const olMatch = line.match(/^\d+[.)]\s+(.+)$/);
    if (olMatch) {
      flushParagraph();
      if (listType !== "ol") {
        flushList();
        listType = "ol";
      }
      listItems.push(olMatch[1].trim());
      continue;
    }

    if (listType && listItems.length > 0) {
      // Continuation line of the preceding list item.
      listItems[listItems.length - 1] += ` ${line}`;
      continue;
    }

    paragraphLines.push(line);
  }

  flushParagraph();
  flushList();
  return blocks.join("");
}
