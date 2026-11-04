-- The poster and backdrop TMDB gives movies and series whose addon gives
-- none, or a dead one, while the settings hold the server's TMDB key: by
-- the identifier the title was looked up with (its TMDB identifier, else
-- its IMDb one) and the server's metadata language they were chosen in,
-- empty for English, for every user. tmdb_id is the title's TMDB
-- identifier, NULL for a title TMDB does not know. poster and backdrop
-- are their paths on TMDB's image server, empty when TMDB has none. A
-- lookup that failed leaves no row.
CREATE TABLE title_artwork (
    kind text NOT NULL CHECK (kind IN ('movie', 'tv')),
    provider text NOT NULL CHECK (provider IN ('imdb', 'tmdb')),
    id text NOT NULL,
    language text NOT NULL DEFAULT '',
    tmdb_id integer CHECK (tmdb_id > 0),
    poster text NOT NULL DEFAULT '',
    backdrop text NOT NULL DEFAULT '',
    asked_at timestamptz NOT NULL,
    PRIMARY KEY (kind, provider, id, language)
);
