-- Ratings from MDBList, asked with the server's own key, empty for none,
-- sealed as the server's other keys are, and which of the critics' and
-- audience scores CriticRating carries.
ALTER TABLE settings
    ADD COLUMN mdblist_ratings_key text NOT NULL DEFAULT '' CHECK (octet_length(mdblist_ratings_key) <= 512 AND mdblist_ratings_key ~ '^[ -~]*$'),
    ADD COLUMN critic_rating_shows text NOT NULL DEFAULT 'critics' CHECK (critic_rating_shows IN ('critics', 'audience'));

-- What MDBList answered for each movie and series, by the identifier it
-- was asked with (an IMDb identifier, else a TMDB one), for every user:
-- the IMDb rating (0 to 10), the critics' and audience scores (0 to 100),
-- each NULL when MDBList has none, and the title's release, which tells
-- how long the answer is kept. found is false for a title MDBList does not
-- know. A request that failed leaves no row: the title is asked about
-- again when it is next listed.
CREATE TABLE title_ratings (
    kind text NOT NULL CHECK (kind IN ('movie', 'show')),
    provider text NOT NULL CHECK (provider IN ('imdb', 'tmdb')),
    id text NOT NULL,
    found boolean NOT NULL,
    imdb real,
    critics real,
    audience real,
    released date,
    asked_at timestamptz NOT NULL,
    PRIMARY KEY (kind, provider, id)
);
