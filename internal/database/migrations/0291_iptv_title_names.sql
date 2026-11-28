-- Whether a source names its movies and series after the metadata addon
-- that describes them, once opened (see package iptv, titlenames.go); it
-- needs enrichment. Off by default.
ALTER TABLE iptv_sources ADD COLUMN metadata_names boolean NOT NULL DEFAULT false;

-- The names a source's titles learned from the metadata addon that
-- described them: key is the title's, or the leader's of a group of
-- titles; search holds the folded name searches match; tmdb and imdb are
-- the identifiers the provider's list gave the title, or the group's
-- TMDB id, when it learned the name. A name goes once the provider no
-- longer lists its title nor does the title lead a group, once the title
-- or its group has other identifiers, and with its source.
CREATE TABLE iptv_title_names (
    addon_id uuid NOT NULL REFERENCES iptv_sources ON DELETE CASCADE,
    type text NOT NULL CHECK (type IN ('movie', 'series')),
    key text NOT NULL CHECK (key <> ''),
    name text NOT NULL CHECK (name <> ''),
    search text NOT NULL,
    tmdb text NOT NULL,
    imdb text NOT NULL,
    PRIMARY KEY (addon_id, type, key)
);
