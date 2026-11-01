-- A library of other videos may show its titles as movies and shows
-- rather than as home videos: a title with videos as a series, one
-- without as a movie. Polyfin only takes it for the catalogs of types it
-- has no use of its own for, and reads it as off for any other.
ALTER TABLE libraries
    ADD COLUMN show_as_titles boolean NOT NULL DEFAULT false;
