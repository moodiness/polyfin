-- The country whose ratings titles show and are judged by, by its ISO
-- 3166-1 alpha-2 code: the US, as before, until an administrator chooses
-- another.
ALTER TABLE settings
    ADD COLUMN rating_country text NOT NULL DEFAULT 'US';
