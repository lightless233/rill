-- Add a non-unique index on bookmarks(item_id) to speed up correlated
-- bookmark_id lookups in ListItems and GetItem without breaking legacy
-- databases that may contain duplicate item_id rows.
CREATE INDEX IF NOT EXISTS idx_bookmarks_item_id ON bookmarks(item_id);
