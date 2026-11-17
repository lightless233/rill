/**
 * Returns undefined when the trimmed draft matches the saved selector, or the
 * trimmed draft value when it changed. A deliberate clear therefore becomes an
 * explicit empty string instead of an omitted field.
 */
export function feedSelectorUpdate(
  saved: string | undefined,
  draft: string,
): string | undefined {
  const normalizedSaved = (saved ?? "").trim();
  const normalizedDraft = draft.trim();
  if (normalizedDraft === normalizedSaved) {
    return undefined;
  }
  return normalizedDraft;
}
