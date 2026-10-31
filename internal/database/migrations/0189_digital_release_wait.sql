-- wait_digital_release hides a movie of the addons' catalogs until it is
-- out at home, as TMDB tells. Off by default.
ALTER TABLE settings
    ADD COLUMN wait_digital_release boolean NOT NULL DEFAULT false;

-- The day each movie is out at home, as TMDB tells, by the identifier it
-- was looked up with (an IMDb identifier, else a TMDB one), for every
-- user: its first digital or physical release in any country, else 90
-- days after it was first shown in cinemas, else anywhere; NULL when TMDB
-- knows no date. A day passed stays, whatever TMDB tells later. found is
-- false for a movie TMDB does not know. A lookup that failed leaves no
-- row.
CREATE TABLE movie_releases (
    provider text NOT NULL CHECK (provider IN ('imdb', 'tmdb')),
    id text NOT NULL,
    found boolean NOT NULL,
    available date,
    asked_at timestamptz NOT NULL,
    PRIMARY KEY (provider, id)
);
