import DOMPurify from "dompurify";
import type { TranslationBlockInput, TranslationMode } from "@/lib/api/types";
import { processArticleContent } from "@/lib/content";
import { toSafeExternalUrl } from "@/lib/safe-url";
import {
  restoreTranslatedMarkup,
  type TranslatedLinkInfo,
} from "./translation-markup";

export type ArticleViewMode = "original" | TranslationMode;

export interface PreparedArticleTranslation {
  sanitizedHtml: string;
  annotatedHtml: string;
  blocks: TranslationBlockInput[];
  linkMaps: Record<number, TranslatedLinkInfo[]>;
}

const purify = DOMPurify(window);

const BLOCK_DESCENDANT_SELECTOR =
  "h1, h2, h3, h4, h5, h6, p, ul, ol, li, div, blockquote, pre, table, details, figure";
const ATOMIC_BLOCK_TAGS = new Set([
  "h1",
  "h2",
  "h3",
  "h4",
  "h5",
  "h6",
  "p",
  "figcaption",
  "summary",
  "th",
  "td",
]);
const INLINE_PASSTHROUGH_TAGS = new Set(["code", "strong", "b", "em", "i"]);
const HAS_LETTER_RE = /\p{L}/u;

function serializeInlineNodes(
  nodes: NodeListOf<ChildNode> | readonly ChildNode[],
  links: TranslatedLinkInfo[],
): string {
  let out = "";
  for (const node of nodes) {
    if (node.nodeType === Node.TEXT_NODE) {
      out += node.textContent ?? "";
      continue;
    }
    if (node.nodeType !== Node.ELEMENT_NODE) {
      continue;
    }
    const el = node as Element;
    const tag = el.tagName.toLowerCase();
    if (tag === "img") {
      continue;
    }
    if (tag === "br") {
      out += "\n";
      continue;
    }
    const inner = serializeInlineNodes(el.childNodes, links);
    if (tag === "a") {
      const safeHref = toSafeExternalUrl(el.getAttribute("href"));
      if (safeHref && inner.trim()) {
        const idx = links.push({ href: safeHref }) - 1;
        out += `<a${idx}>${inner}</a${idx}>`;
      } else {
        out += inner;
      }
      continue;
    }
    if (INLINE_PASSTHROUGH_TAGS.has(tag)) {
      if (inner) {
        out += `<${tag}>${inner}</${tag}>`;
      }
      continue;
    }
    out += inner;
  }
  return out;
}

export function prepareArticleTranslation(
  html: string,
  articleUrl?: string,
): PreparedArticleTranslation {
  const sanitizedHtml = processArticleContent(html, articleUrl);
  const root = document.createElement("div");
  root.innerHTML = sanitizedHtml;

  const blocks: TranslationBlockInput[] = [];
  const linkMaps: Record<number, TranslatedLinkInfo[]> = {};

  const registerElement = (el: Element) => {
    const plain = (el.textContent ?? "").trim();
    if (!plain || !HAS_LETTER_RE.test(plain)) {
      return;
    }
    const links: TranslatedLinkInfo[] = [];
    const serialized = serializeInlineNodes(el.childNodes, links)
      .replace(/[ \t]+\n/g, "\n")
      .replace(/\n[ \t]+/g, "\n")
      .trim();
    if (!serialized || !HAS_LETTER_RE.test(serialized)) {
      return;
    }
    const id = blocks.length;
    el.setAttribute("data-rill-tb", String(id));
    blocks.push({ id, text: serialized });
    if (links.length > 0) {
      linkMaps[id] = links;
    }
  };

  const wrapLooseInlineSegment = (
    parent: Element,
    runs: ChildNode[],
  ) => {
    if (runs.length === 0) return;
    const combinedText = runs.map((n) => n.textContent ?? "").join("").trim();
    if (!combinedText || !HAS_LETTER_RE.test(combinedText)) {
      runs.length = 0;
      return;
    }
    const span = document.createElement("span");
    span.setAttribute("data-rill-inline-wrap", "true");
    parent.insertBefore(span, runs[0]);
    for (const n of runs) {
      span.appendChild(n);
    }
    runs.length = 0;
    registerElement(span);
  };

  const visitContainerChildren = (container: Element) => {
    const pendingInline: ChildNode[] = [];
    const children = Array.from(container.childNodes);
    for (const child of children) {
      if (child.nodeType === Node.TEXT_NODE) {
        pendingInline.push(child);
        continue;
      }
      if (child.nodeType !== Node.ELEMENT_NODE) {
        continue;
      }
      const el = child as Element;
      const tag = el.tagName.toLowerCase();
      if (
        ATOMIC_BLOCK_TAGS.has(tag) ||
        BLOCK_DESCENDANT_SELECTOR.split(", ").includes(tag) ||
        tag === "thead" ||
        tag === "tbody" ||
        tag === "tr"
      ) {
        wrapLooseInlineSegment(container, pendingInline);
        visitElement(el);
      } else {
        pendingInline.push(child);
      }
    }
    wrapLooseInlineSegment(container, pendingInline);
  };

  const visitElement = (el: Element) => {
    const tag = el.tagName.toLowerCase();
    if (tag === "pre") {
      return;
    }
    if (ATOMIC_BLOCK_TAGS.has(tag)) {
      registerElement(el);
      return;
    }
    if (tag === "blockquote" || tag === "li" || tag === "div" || tag === "section") {
      if (!el.querySelector(BLOCK_DESCENDANT_SELECTOR)) {
        registerElement(el);
        return;
      }
      visitContainerChildren(el);
      return;
    }
    for (const child of Array.from(el.children)) {
      visitElement(child);
    }
  };

  visitContainerChildren(root);

  return {
    sanitizedHtml,
    annotatedHtml: root.innerHTML,
    blocks,
    linkMaps,
  };
}

function createTranslationElement(
  tag: "p" | "div" | "span",
  className: string,
  safeInnerHtml: string,
): HTMLElement {
  const node = document.createElement(tag);
  node.className = className;
  node.innerHTML = purify.sanitize(safeInnerHtml, {
    ALLOWED_TAGS: ["a", "code", "strong", "em", "b", "i", "br"],
    ALLOWED_ATTR: ["href", "target", "rel"],
    ALLOW_DATA_ATTR: false,
  });
  return node;
}

export function renderTranslatedArticleHtml(
  prepared: PreparedArticleTranslation,
  translations: Readonly<Record<number, string>>,
  mode: ArticleViewMode,
): string {
  if (mode === "original") {
    return prepared.sanitizedHtml;
  }

  const root = document.createElement("div");
  root.innerHTML = prepared.annotatedHtml;

  const targets = Array.from(root.querySelectorAll("[data-rill-tb]"));
  for (const el of targets) {
    // Keep the block id in the DOM so viewport-driven translation can observe
    // which blocks need more work as the reader scrolls.
    const rawId = el.getAttribute("data-rill-tb");
    if (rawId === null) continue;

    const id = Number.parseInt(rawId, 10);
    const translated = translations[id];
    if (!translated || !translated.trim()) {
      continue;
    }

    const safeHtml = restoreTranslatedMarkup(
      translated,
      prepared.linkMaps[id] ?? [],
    );

    if (mode === "zh") {
      const preservedImages = Array.from(el.querySelectorAll("img"));
      el.innerHTML = purify.sanitize(safeHtml, {
        ALLOWED_TAGS: ["a", "code", "strong", "em", "b", "i", "br"],
        ALLOWED_ATTR: ["href", "target", "rel"],
        ALLOW_DATA_ATTR: false,
      });
      if (preservedImages.length > 0) {
        const prefix = document.createDocumentFragment();
        for (const img of preservedImages) {
          prefix.appendChild(img);
        }
        el.prepend(prefix);
      }
      continue;
    }

    const tag = el.tagName.toLowerCase();
    if (tag === "th" || tag === "td") {
      el.appendChild(
        createTranslationElement("div", "rill-translation-cell", safeHtml),
      );
      continue;
    }

    if (tag === "summary") {
      el.appendChild(
        createTranslationElement("span", "rill-translation-inline", safeHtml),
      );
      continue;
    }

    if (tag === "li") {
      const block = createTranslationElement(
        "div",
        "rill-translation-block",
        safeHtml,
      );
      const nestedList = el.querySelector(":scope > ul, :scope > ol");
      if (nestedList) {
        el.insertBefore(block, nestedList);
      } else {
        el.appendChild(block);
      }
      continue;
    }

    if (tag === "blockquote" || tag === "figcaption") {
      el.appendChild(
        createTranslationElement(
          tag === "blockquote" ? "p" : "div",
          "rill-translation-block",
          safeHtml,
        ),
      );
      continue;
    }

    if (/^h[1-6]$/.test(tag)) {
      const heading = createTranslationElement(
        "p",
        `rill-translation-heading rill-translation-${tag}`,
        safeHtml,
      );
      el.after(heading);
      continue;
    }

    const block = createTranslationElement(
      tag === "span" ? "div" : "p",
      "rill-translation-block",
      safeHtml,
    );
    el.after(block);
  }

  return root.innerHTML;
}
