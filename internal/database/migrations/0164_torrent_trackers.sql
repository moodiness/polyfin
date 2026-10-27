-- The announce URLs of trackers asked for every torrent besides those its
-- addon names, none by default: Polyfin ships no list.
ALTER TABLE settings
    ADD COLUMN torrent_trackers text[] NOT NULL DEFAULT '{}' CHECK (cardinality(torrent_trackers) <= 30);
