-- The TMDB collections movies belong to, which Silo's apps show as box
-- sets: each collection's name, and the paths of its poster and backdrop
-- on TMDB's image server, empty when TMDB has none, as a movie's details
-- name them.
CREATE TABLE tmdb_collections (
    id bigint PRIMARY KEY CHECK (id > 0),
    name text NOT NULL,
    poster_path text NOT NULL DEFAULT '',
    backdrop_path text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL
);

-- The collection each movie belongs to, as TMDB tells, by the identifier
-- it was looked up with (its TMDB identifier, else its IMDb one), for every
-- user: NULL for a movie in none. found is false for a movie TMDB does not
-- know. A lookup that failed leaves no row.
CREATE TABLE movie_collections (
    provider text NOT NULL CHECK (provider IN ('imdb', 'tmdb')),
    id text NOT NULL,
    found boolean NOT NULL,
    collection_id bigint REFERENCES tmdb_collections,
    asked_at timestamptz NOT NULL,
    PRIMARY KEY (provider, id)
);
