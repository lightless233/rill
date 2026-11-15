-- Persist explicit folder ordering for groups.

ALTER TABLE groups ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0;

UPDATE groups SET sort_order = id;
