-- Per-article AI summary cache sharing the configured DeepSeek/OpenAI endpoint.

CREATE TABLE IF NOT EXISTS article_summaries (
	content_hash TEXT NOT NULL,
	target_lang  TEXT NOT NULL DEFAULT 'zh-CN',
	source_title TEXT NOT NULL DEFAULT '',
	summary      TEXT NOT NULL,
	model        TEXT NOT NULL DEFAULT '',
	created_at   INTEGER NOT NULL DEFAULT (unixepoch()),
	updated_at   INTEGER NOT NULL DEFAULT (unixepoch()),
	PRIMARY KEY (content_hash, target_lang)
);
