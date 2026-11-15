-- Typography preferences for article titles and reading body. Nullable with
-- no DEFAULT for the same reason as auto_mark_read: NULL means "the user has
-- never explicitly chosen a font" so existing databases keep their client
-- fallback unless set. Validated against handler allow-lists.

ALTER TABLE settings ADD COLUMN latin_font TEXT;
ALTER TABLE settings ADD COLUMN cjk_font TEXT;
ALTER TABLE settings ADD COLUMN mono_font TEXT;
