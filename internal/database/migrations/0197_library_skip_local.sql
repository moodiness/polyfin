-- A library of movies or series may leave out the titles a local folder
-- holds: those a folder's library lists by the same identifier. The
-- folder's library lists them, and the title stays what it was.
ALTER TABLE libraries
    ADD COLUMN skip_local boolean NOT NULL DEFAULT false
        CHECK (NOT skip_local OR catalog_type IN ('movie', 'anime.movie', 'series', 'anime.series', 'anime'));
