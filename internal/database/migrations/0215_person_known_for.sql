-- The IMDb identifier TMDB gives in the external identifiers of a movie or
-- series, by its TMDB identifier, asked for the titles a person is known
-- for that the users' addons describe by IMDb identifier only. imdb_id is
-- NULL when TMDB gives none. A lookup that failed leaves no row.
CREATE TABLE title_imdb_ids (
    kind text NOT NULL CHECK (kind IN ('movie', 'tv')),
    tmdb_id integer NOT NULL CHECK (tmdb_id > 0),
    imdb_id text CHECK (imdb_id ~ '^tt[0-9]+$'),
    asked_at timestamptz NOT NULL,
    PRIMARY KEY (kind, tmdb_id)
);

-- People's combined credits now keep, for each title, its name, release
-- date, poster's path, popularity and vote count, and whether TMDB marks
-- it as an appearance rather than a role, or as for adults ("aside"),
-- which rank the titles a person is known for: a list of {"kind", "id",
-- "name", "date", "poster", "votes", "popularity", "aside"}. Those kept
-- without them are asked again when a person's page opens.
DELETE FROM person_credits;
