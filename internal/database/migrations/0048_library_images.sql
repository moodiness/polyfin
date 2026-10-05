-- The image Jellyfin apps show on a library's tile: none, as before, or one
-- found in its catalog ('automatic'). An image uploaded for the library's
-- item (item_images, Primary) wins over both. Live TV catalogs make no
-- library: they have none.
ALTER TABLE libraries
    ADD COLUMN image text NOT NULL DEFAULT 'none'
        CHECK (image IN ('none', 'automatic') AND (image = 'none' OR catalog_type <> 'tv'));
