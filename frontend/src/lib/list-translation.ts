import type {
  Item,
  ListTranslationItemRequest,
  ListTranslationMode,
} from "@/lib/api";
import { extractTranslationSource } from "@/lib/utils";

export interface ListTranslationText {
  title?: string;
  summary?: string;
}

export interface ListTranslationInput extends ListTranslationItemRequest {
  key: string;
}

export const LIST_TRANSLATION_BATCH_SIZE = 20;
export const LIST_TRANSLATION_SOURCE_LENGTH = 400;

const LATIN_RE = /[A-Za-z]/;
const CJK_RE = /[\u3400-\u9fff]/;

/**
 * List translation targets foreign-language fields only. Mixed Chinese/English
 * text is already readable enough and is left untouched to avoid unnecessary
 * model calls.
 */
export function isEligibleForTranslation(text: string): boolean {
  return LATIN_RE.test(text) && !CJK_RE.test(text);
}

export function effectiveListTranslationMode(
  mode: ListTranslationMode | null,
  hasKey: boolean,
): ListTranslationMode {
  if (!hasKey) return "original";
  return mode ?? "bilingual";
}

export function buildListTranslationInput(
  article: Item,
): ListTranslationInput | null {
  const title = article.title.trim();
  const eligibleTitle = isEligibleForTranslation(title) ? title : "";
  const source = extractTranslationSource(
    article.content,
    LIST_TRANSLATION_SOURCE_LENGTH,
  );
  const eligibleSummary = isEligibleForTranslation(source) ? source : "";

  if (!eligibleTitle && !eligibleSummary) {
    return null;
  }

  return {
    item_id: article.id,
    ...(eligibleTitle ? { title: eligibleTitle } : {}),
    ...(eligibleSummary ? { summary: eligibleSummary } : {}),
    key: `${article.id}\u0000${eligibleTitle}\u0000${eligibleSummary}`,
  };
}

export function listTranslationSignature(
  inputs: readonly ListTranslationInput[],
): string {
  return inputs.map((input) => input.key).join("\u0001");
}
