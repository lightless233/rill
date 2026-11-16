-- Time-limited public article snapshots. The row owns its content so deleting
-- the source item or feed does not break an active share. Expired rows are
-- scrubbed of content/passcode but keep token/expires_at as a tombstone for a
-- precise 410 response.

CREATE TABLE IF NOT EXISTS article_shares (
	id               INTEGER PRIMARY KEY,
	token            TEXT NOT NULL UNIQUE,
	item_id          INTEGER REFERENCES items(id) ON UPDATE CASCADE ON DELETE SET NULL,
	title            TEXT NOT NULL DEFAULT '',
	content          TEXT NOT NULL DEFAULT '',
	link             TEXT NOT NULL DEFAULT '',
	feed_name        TEXT NOT NULL DEFAULT '',
	pub_date         INTEGER NOT NULL DEFAULT 0,
	passcode_hash    TEXT,
	expires_at       INTEGER NOT NULL,
	created_at       INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE INDEX IF NOT EXISTS idx_article_shares_expires_at ON article_shares(expires_at);
