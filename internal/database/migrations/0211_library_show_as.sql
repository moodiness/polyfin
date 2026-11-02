-- How a library shows its catalog's titles, as an administrator or a
-- user chose it, or as Polyfin decided it from the catalog's first page:
-- 'home_videos' or 'movies_and_shows' for a catalog of other videos,
-- 'listed' or 'movies_and_shows' for a catalog of movies or series. NULL
-- is not chosen yet: a catalog of other videos is shown as home videos
-- until Polyfin reads its first page, a catalog of movies or series as
-- listed. Polyfin reads a choice the catalog no longer takes, after its
-- type changed, as none. The libraries shown as movies and shows keep it.
ALTER TABLE libraries
    ADD COLUMN show_as text CHECK (show_as IN ('home_videos', 'listed', 'movies_and_shows'));

UPDATE libraries SET show_as = 'movies_and_shows' WHERE show_as_titles;

ALTER TABLE libraries
    DROP COLUMN show_as_titles;
