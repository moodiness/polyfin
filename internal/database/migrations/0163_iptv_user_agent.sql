-- The User-Agent an IPTV source's list and streams are requested with,
-- as its administrator set it: empty for the default, a browser's for the
-- streams of M3U playlists and Xtream accounts (see package iptv).
ALTER TABLE iptv_sources
    ADD COLUMN user_agent text NOT NULL DEFAULT '';
