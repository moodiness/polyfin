-- A library of an anime catalog may show its titles as shows or as
-- movies, besides movies and shows: in a library of that type, which
-- apps then list one kind of title only. NULL stays movies and shows.
ALTER TABLE libraries
    DROP CONSTRAINT libraries_show_as_check;

ALTER TABLE libraries
    ADD CONSTRAINT libraries_show_as_check
        CHECK (show_as IN ('home_videos', 'listed', 'movies_and_shows', 'shows', 'movies'));
