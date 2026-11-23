-- The order apps list a collection made by users in: by premiere date, as
-- Jellyfin orders a collection, or in the order an administrator chose in
-- the admin app, that of collection_items.ordinal.
ALTER TABLE collections ADD COLUMN display_order text NOT NULL DEFAULT 'premiere'
    CHECK (display_order IN ('premiere', 'chosen'));
