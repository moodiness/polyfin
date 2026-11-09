-- TMDB's IMDb identifier of a TMDB title now tells a title TMDB does not
-- know (found false, imdb_id NULL) from one it gives no IMDb identifier
-- for (found true, imdb_id NULL). The rows without an IMDb identifier
-- kept before could be either: they go, to be asked again. The answers of
-- TMDB's find by IMDb identifier are kept here too, as the same fact, and
-- read by IMDb identifier.
ALTER TABLE title_imdb_ids ADD COLUMN found boolean NOT NULL DEFAULT true;
ALTER TABLE title_imdb_ids ADD CONSTRAINT title_imdb_ids_found CHECK (found OR imdb_id IS NULL);
DELETE FROM title_imdb_ids WHERE imdb_id IS NULL;
CREATE INDEX title_imdb_ids_imdb ON title_imdb_ids (imdb_id) WHERE imdb_id IS NOT NULL;

-- The checks of the TMDB identifiers metadata addons give as moviedb_id,
-- each kept for good: the identifier tmdb_id of a title of kind, of the
-- IMDb identifier imdb_id, else of none ('') and of year (0 when unknown,
-- and with an IMDb identifier), is wrong or not. A wrong one is replaced
-- by TMDB's identifier of the title (replaced_by), else dropped. Another
-- identifier an addon gives later is checked on its own.
CREATE TABLE moviedb_id_checks (
    kind text NOT NULL CHECK (kind IN ('movie', 'tv')),
    tmdb_id integer NOT NULL CHECK (tmdb_id > 0),
    imdb_id text NOT NULL CHECK (imdb_id = '' OR imdb_id ~ '^tt[0-9]+$'),
    year integer NOT NULL CHECK (year >= 0),
    wrong boolean NOT NULL,
    replaced_by integer CHECK (replaced_by IS NULL OR replaced_by > 0 AND wrong),
    checked_at timestamptz NOT NULL,
    PRIMARY KEY (kind, tmdb_id, imdb_id, year)
);
