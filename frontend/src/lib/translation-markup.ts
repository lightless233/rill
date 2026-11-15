export interface TranslatedLinkInfo {
  href: string;
}

const ALLOWED_PLACEHOLDER_TAG_RE =
  /<(\/?)(a(\d+)|code|strong|em|b|i|br)\s*(\/?)>/gi;

export function escapeHtmlText(text: string): string {
  return text
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");
}

function escapeHtmlAttr(value: string): string {
  return escapeHtmlText(value).replaceAll('"', "&quot;").replaceAll("'", "&#39;");
}

/**
 * Converts translated segment text containing <aN> / inline placeholders back
 * into safe HTML. Any unrecognized markup or angle brackets are escaped as
 * plain text, while <aN> links are restored strictly from verified link maps.
 */
export function restoreTranslatedMarkup(
  rawTranslated: string,
  links: readonly TranslatedLinkInfo[],
): string {
  if (!rawTranslated) return "";

  let out = "";
  let lastIndex = 0;
  const openLinkIndices: number[] = [];

  for (const match of rawTranslated.matchAll(ALLOWED_PLACEHOLDER_TAG_RE)) {
    const index = match.index ?? 0;
    if (index > lastIndex) {
      out += escapeHtmlText(rawTranslated.slice(lastIndex, index));
    }
    lastIndex = index + match[0].length;

    const isClosing = match[1] === "/";
    const rawTag = match[2].toLowerCase();
    const linkIndexText = match[3];
    const isSelfClosing = match[4] === "/";

    if (rawTag === "br") {
      if (!isClosing) {
        out += "<br />";
      }
      continue;
    }

    if (linkIndexText !== undefined) {
      const linkIndex = Number.parseInt(linkIndexText, 10);
      if (isClosing) {
        if (openLinkIndices.includes(linkIndex)) {
          const stackIdx = openLinkIndices.lastIndexOf(linkIndex);
          openLinkIndices.splice(stackIdx, 1);
          out += "</a>";
        }
        continue;
      }

      const info = links[linkIndex];
      if (!info || isSelfClosing) {
        continue;
      }
      openLinkIndices.push(linkIndex);
      out += `<a href="${escapeHtmlAttr(info.href)}" target="_blank" rel="noopener noreferrer">`;
      continue;
    }

    if (isSelfClosing) {
      continue;
    }
    out += `<${isClosing ? "/" : ""}${rawTag}>`;
  }

  if (lastIndex < rawTranslated.length) {
    out += escapeHtmlText(rawTranslated.slice(lastIndex));
  }

  while (openLinkIndices.length > 0) {
    openLinkIndices.pop();
    out += "</a>";
  }

  return out;
}
