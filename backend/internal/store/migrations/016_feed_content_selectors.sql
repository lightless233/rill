-- Per-feed CSS extraction rules. Empty content_selector disables extraction;
-- exclude_selector is optional and has no effect while extraction is disabled.
-- Existing rows and newly created feeds inherit the empty defaults.

ALTER TABLE feeds ADD COLUMN content_selector TEXT NOT NULL DEFAULT '';
ALTER TABLE feeds ADD COLUMN exclude_selector TEXT NOT NULL DEFAULT '';
