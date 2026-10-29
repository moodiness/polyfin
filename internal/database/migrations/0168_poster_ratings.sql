-- The web player shows a title's ratings on its poster as the pointer or
-- the focus comes to it (see /Polyfin/Items/Ratings), off by default.
ALTER TABLE settings
    ADD COLUMN poster_ratings boolean NOT NULL DEFAULT false;
