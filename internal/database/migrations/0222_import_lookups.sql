-- Imports, recommendations and marks find the movies and series Polyfin
-- listed by their IMDb, TMDB and TVDB identifiers (besides their key), and
-- the episodes listed of a series by its Stremio identifier: these indexes
-- spare those lookups a scan of every item.
CREATE INDEX items_title_imdb ON items ((data->'meta'->>'imdb_id')) WHERE kind IN ('movie', 'series');
CREATE INDEX items_title_tmdb ON items ((data->'meta'->>'_tmdbId')) WHERE kind IN ('movie', 'series');
CREATE INDEX items_title_tvdb ON items ((data->'meta'->>'_tvdbId')) WHERE kind IN ('movie', 'series');
CREATE INDEX items_episode_series ON items ((data->>'seriesId')) WHERE kind = 'episode';

-- PostgreSQL takes no statistics from a partial index: without these, it
-- guesses that a few hundred identifiers match most titles, and reads
-- them all instead of looking each one up.
CREATE STATISTICS items_title_imdb_values ON ((data->'meta'->>'imdb_id')) FROM items;
CREATE STATISTICS items_title_tmdb_values ON ((data->'meta'->>'_tmdbId')) FROM items;
CREATE STATISTICS items_title_tvdb_values ON ((data->'meta'->>'_tvdbId')) FROM items;
CREATE STATISTICS items_episode_series_values ON ((data->>'seriesId')) FROM items;
ANALYZE items;
