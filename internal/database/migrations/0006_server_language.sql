-- The language of the names Polyfin generates for Jellyfin apps: seasons,
-- episodes without a title and the suffixes that tell libraries apart.
ALTER TABLE settings
    ADD COLUMN language text NOT NULL DEFAULT 'en' CHECK (language IN ('en', 'fr'));
