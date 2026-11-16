-- Per-block cache for lazy article-body translation. The frontend only sends
-- blocks near the viewport, so a whole-article cache row cannot represent
-- partial progress. Hashing the normalized block text lets a translated block
-- be reused across reopenings (and across articles) without re-calling the
-- model; target_lang keeps the cache language-scoped.
--
-- Title translations use the same table with a title-specific hash, keeping
-- partial article progress and title progress independent.

CREATE TABLE IF NOT EXISTS article_block_translations (
	content_hash    TEXT NOT NULL,
	target_lang     TEXT NOT NULL,
	source_text     TEXT NOT NULL DEFAULT '',
	translated_text TEXT NOT NULL,
	model           TEXT NOT NULL DEFAULT '',
	created_at      INTEGER NOT NULL DEFAULT (unixepoch()),
	updated_at      INTEGER NOT NULL DEFAULT (unixepoch()),
	PRIMARY KEY (content_hash, target_lang)
);
