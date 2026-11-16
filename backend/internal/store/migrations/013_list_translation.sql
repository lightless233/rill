-- Article list translation mode. NULL keeps its existing meaning: the user has
-- never chosen a value, so the client derives the effective default from whether
-- a translation API key is configured. "original" is an explicit opt-out.
--
-- The settings table stays single-row and typed; adding a preference as one
-- nullable column is intentional for this small closed set. A key-value or JSON
-- settings refactor is a separate change, not part of adding this preference.

ALTER TABLE settings ADD COLUMN list_translation_mode TEXT;
