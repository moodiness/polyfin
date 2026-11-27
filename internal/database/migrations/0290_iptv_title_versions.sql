-- Whether an Xtream source makes the movies, or the series, that its lists
-- give more than once with the same TMDB id one title, whose versions are
-- each entry's file (see package iptv, groups.go). Off by default.
ALTER TABLE iptv_sources ADD COLUMN title_versions boolean NOT NULL DEFAULT false;

-- A source's titles of a type grouped by their TMDB id. leader is the key
-- of the title whose identifiers the group takes, kept while the group
-- lasts, even once the provider no longer lists it; face is the key of
-- the entry whose list data describes the group and whose category lists
-- it: the leader while it is listed, else the member that joined first.
CREATE TABLE iptv_title_groups (
    addon_id uuid NOT NULL REFERENCES iptv_sources ON DELETE CASCADE,
    type text NOT NULL CHECK (type IN ('movie', 'series')),
    tmdb text NOT NULL CHECK (tmdb <> ''),
    leader text NOT NULL CHECK (leader <> ''),
    face text NOT NULL CHECK (face <> ''),
    PRIMARY KEY (addon_id, type, tmdb),
    UNIQUE (addon_id, type, leader)
);

-- The titles of each group, in the order they joined it (position): its
-- leader, listed or not, and the entries the provider lists now (listed).
CREATE TABLE iptv_title_members (
    addon_id uuid NOT NULL,
    type text NOT NULL,
    key text NOT NULL CHECK (key <> ''),
    tmdb text NOT NULL,
    position integer NOT NULL CHECK (position >= 0),
    listed boolean NOT NULL,
    PRIMARY KEY (addon_id, type, key),
    FOREIGN KEY (addon_id, type, tmdb) REFERENCES iptv_title_groups ON DELETE CASCADE
);
CREATE INDEX iptv_title_members_group ON iptv_title_members (addon_id, type, tmdb, position);

-- The titles an administrator keeps apart from the others of their TMDB
-- id, whose id the provider got wrong; kept while the option is off.
CREATE TABLE iptv_titles_apart (
    addon_id uuid NOT NULL REFERENCES iptv_sources ON DELETE CASCADE,
    type text NOT NULL CHECK (type IN ('movie', 'series')),
    key text NOT NULL CHECK (key <> ''),
    PRIMARY KEY (addon_id, type, key)
);

-- The titles of a TMDB id, which grouping and the Versions page look up.
CREATE INDEX iptv_titles_tmdb ON iptv_titles (addon_id, type, tmdb) WHERE tmdb <> '';
