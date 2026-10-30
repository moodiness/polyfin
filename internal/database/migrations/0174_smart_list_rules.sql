-- Smart lists' exclusions, a group of rules whose titles a list leaves
-- out, and their sorts by the latest episode and the latest season
-- premiere aired.
ALTER TABLE smart_lists
    ADD COLUMN exclude jsonb NOT NULL DEFAULT '{"field": "group", "match": "any"}',
    DROP CONSTRAINT smart_lists_sort_check,
    ADD CONSTRAINT smart_lists_sort_check
        CHECK (sort IN ('catalog', 'premiere', 'latestEpisode', 'seasonPremiere', 'rating', 'name', 'random'));

-- How many votes the IMDb rating MDBList gives counts, NULL for none. The
-- titles kept before are asked about again as they next show, for theirs.
ALTER TABLE title_ratings
    ADD COLUMN imdb_votes integer CHECK (imdb_votes > 0);
UPDATE title_ratings SET asked_at = asked_at - interval '400 days' WHERE found;

-- The studios that made each movie and series, and the networks a series
-- aired on, as TMDB names them, by the identifier the title was looked up
-- with (an IMDb identifier, else a TMDB one), for every user. found is
-- false for a title TMDB does not know. A lookup that failed leaves no row.
CREATE TABLE title_studios (
    kind text NOT NULL CHECK (kind IN ('movie', 'tv')),
    provider text NOT NULL CHECK (provider IN ('imdb', 'tmdb')),
    id text NOT NULL,
    found boolean NOT NULL,
    studios text[] NOT NULL DEFAULT '{}',
    asked_at timestamptz NOT NULL,
    PRIMARY KEY (kind, provider, id)
);
