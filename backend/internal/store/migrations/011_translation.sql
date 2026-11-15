-- DeepSeek article translation configuration and per-article translation cache.
-- Note: translation_api_key stores the secret server-side and is never echoed in
-- plaintext over HTTP. Empty string represents an explicitly cleared field.

ALTER TABLE settings ADD COLUMN translation_api_url TEXT;
ALTER TABLE settings ADD COLUMN translation_api_key TEXT;
ALTER TABLE settings ADD COLUMN translation_model TEXT;
ALTER TABLE settings ADD COLUMN translation_mode TEXT;

CREATE TABLE IF NOT EXISTS article_translations (
	content_hash     TEXT NOT NULL,
	target_lang      TEXT NOT NULL DEFAULT 'zh-CN',
	source_title     TEXT NOT NULL DEFAULT '',
	translated_title TEXT NOT NULL DEFAULT '',
	blocks_json      TEXT NOT NULL,
	model            TEXT NOT NULL DEFAULT '',
	created_at       INTEGER NOT NULL DEFAULT (unixepoch()),
	updated_at       INTEGER NOT NULL DEFAULT (unixepoch()),
	PRIMARY KEY (content_hash, target_lang)
);
