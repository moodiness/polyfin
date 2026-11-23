-- Smart lists' sort by the date the user last played a title, the latest
-- first.
ALTER TABLE smart_lists
    DROP CONSTRAINT smart_lists_sort_check,
    ADD CONSTRAINT smart_lists_sort_check
        CHECK (sort IN ('catalog', 'premiere', 'latestEpisode', 'seasonPremiere', 'rating', 'name', 'random', 'datePlayed'));

-- The Metacritic score MDBList gives (0 to 100), NULL for none. The titles
-- kept before are asked about again as they next show, for theirs.
ALTER TABLE title_ratings
    ADD COLUMN metacritic real;
UPDATE title_ratings SET asked_at = asked_at - interval '400 days' WHERE found;
