-- Narrow items_fts update trigger so read-state flips and unchanged
-- title/content updates do not churn the FTS index.

DROP TRIGGER IF EXISTS items_fts_items_au;

CREATE TRIGGER IF NOT EXISTS items_fts_items_au
AFTER UPDATE OF title, content ON items
WHEN OLD.title IS NOT NEW.title OR OLD.content IS NOT NEW.content
BEGIN
	DELETE FROM items_fts WHERE rowid = OLD.id;
	INSERT INTO items_fts(rowid, title, content)
	VALUES (NEW.id, NEW.title, NEW.content);
END;
