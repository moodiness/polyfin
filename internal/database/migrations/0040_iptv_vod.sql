-- What an IPTV source imports: its live channels (the line-up), the
-- provider's movies and series; the movie and series categories left out
-- (movie:<name>, series:<name>); whether its VOD libraries are one per type
-- or one per category; and whether titles with a provider TMDB or IMDb id
-- are described through the server's metadata addons.
ALTER TABLE iptv_sources
    ADD COLUMN live_tv boolean NOT NULL DEFAULT true,
    ADD COLUMN movies boolean NOT NULL DEFAULT false,
    ADD COLUMN series boolean NOT NULL DEFAULT false,
    ADD COLUMN vod_excluded text[] NOT NULL DEFAULT '{}' CHECK (cardinality(vod_excluded) <= 5000),
    ADD COLUMN vod_libraries text NOT NULL DEFAULT 'type' CHECK (vod_libraries IN ('type', 'category')),
    ADD COLUMN enrichment boolean NOT NULL DEFAULT true,
    ADD CONSTRAINT iptv_sources_imports_something CHECK (live_tv OR movies OR series);

-- What an M3U entry is, told by its address or name: a live channel, a
-- movie, or an episode of the series named series_name, with its season
-- and episode numbers.
ALTER TABLE iptv_entries
    ADD COLUMN kind text NOT NULL DEFAULT 'live' CHECK (kind IN ('live', 'movie', 'episode')),
    ADD COLUMN series_name text NOT NULL DEFAULT '',
    ADD COLUMN season integer CHECK (season >= 0),
    ADD COLUMN episode integer CHECK (episode >= 0);

-- The movies and series of a source's lists, in the provider's order. key
-- is the provider's identifier (an Xtream stream or series id, the id in an
-- M3U address, else a hash of the name), which the title's Stremio and
-- Jellyfin identifiers are made of. category is the provider's category
-- name; added the date the provider added it; extension the container of
-- an Xtream movie's file; quality the label its name gives (4K, FHD, HD,
-- SD…); url and headers an M3U movie's stream; listing
-- what the list says beyond that (an Xtream series' plot, cast…); version
-- what tells its details changed (an Xtream series' last modification).
-- search holds the folded name searches match.
CREATE TABLE iptv_titles (
    addon_id uuid NOT NULL REFERENCES iptv_sources ON DELETE CASCADE,
    type text NOT NULL CHECK (type IN ('movie', 'series')),
    key text NOT NULL CHECK (key <> ''),
    position integer NOT NULL CHECK (position > 0),
    name text NOT NULL CHECK (name <> ''),
    category text NOT NULL DEFAULT '',
    poster text NOT NULL DEFAULT '',
    year integer CHECK (year BETWEEN 1800 AND 3000),
    rating text NOT NULL DEFAULT '',
    added timestamptz,
    extension text NOT NULL DEFAULT '',
    quality text NOT NULL DEFAULT '',
    tmdb text NOT NULL DEFAULT '',
    imdb text NOT NULL DEFAULT '',
    url text,
    headers jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(headers) = 'object'),
    listing jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(listing) = 'object'),
    version text NOT NULL DEFAULT '',
    search text NOT NULL DEFAULT '',
    PRIMARY KEY (addon_id, type, key)
);
CREATE INDEX iptv_titles_order ON iptv_titles (addon_id, type, added DESC NULLS LAST, position);
CREATE INDEX iptv_titles_category ON iptv_titles (addon_id, type, category);

-- The episodes of an M3U source's series, by the series' key.
CREATE TABLE iptv_episodes (
    addon_id uuid NOT NULL REFERENCES iptv_sources ON DELETE CASCADE,
    series_key text NOT NULL,
    key text NOT NULL CHECK (key <> ''),
    position integer NOT NULL CHECK (position > 0),
    season integer NOT NULL CHECK (season >= 0),
    episode integer NOT NULL CHECK (episode >= 0),
    name text NOT NULL DEFAULT '',
    quality text NOT NULL DEFAULT '',
    url text NOT NULL CHECK (url <> ''),
    headers jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(headers) = 'object'),
    PRIMARY KEY (addon_id, series_key, key)
);

-- The details an Xtream server gave for a title when it was opened or
-- played (get_vod_info, get_series_info), kept across list refreshes and
-- asked again once version, the title's, changed or they grew old.
CREATE TABLE iptv_details (
    addon_id uuid NOT NULL REFERENCES iptv_sources ON DELETE CASCADE,
    type text NOT NULL CHECK (type IN ('movie', 'series')),
    key text NOT NULL,
    version text NOT NULL DEFAULT '',
    fetched_at timestamptz NOT NULL,
    details jsonb NOT NULL CHECK (jsonb_typeof(details) = 'object'),
    PRIMARY KEY (addon_id, type, key)
);
