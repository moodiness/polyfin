-- The titles an import kept that it did not find, per user and import
-- source (see importlinks): 'nuvio', 'stremio', a tracking service's name,
-- 'letterboxd', 'jellyfin', 'emby' or 'plex'. entry_key names the title
-- the way the source does, stable from one import to the next: the
-- series' for an episode, which season and episode place. The label shows
-- it to the user: its kind, its name, empty when the source gives none,
-- its year, null when unknown, the identifiers the source names it by, and
-- why it was not found. The rest is what the entry would import, so that a
-- link applies it at once: played, at played_at, plays times; a resume
-- point at resume_percent, or resume_position_ms of resume_runtime_ms, set
-- at resume_at; the watchlist list it goes on, null when none, added at
-- added_at; and the rating from 0 to 10, null when none. seen_at is when
-- an import last kept it.
CREATE TABLE import_unfound (
    user_id uuid NOT NULL REFERENCES users ON DELETE CASCADE,
    source text NOT NULL CHECK (source ~ '^[a-z]+$'),
    entry_key text NOT NULL CHECK (entry_key <> ''),
    season integer NOT NULL DEFAULT 0 CHECK (season >= 0),
    episode integer NOT NULL DEFAULT 0 CHECK (episode >= 0),
    kind text NOT NULL CHECK (kind IN ('movie', 'series', 'episode')),
    name text NOT NULL DEFAULT '',
    year integer CHECK (year > 0),
    ids text NOT NULL DEFAULT '',
    reason text NOT NULL CHECK (reason <> ''),
    played boolean NOT NULL DEFAULT false,
    played_at timestamptz,
    plays integer NOT NULL DEFAULT 0 CHECK (plays >= 0),
    resume_percent double precision CHECK (resume_percent >= 0),
    resume_position_ms bigint CHECK (resume_position_ms >= 0),
    resume_runtime_ms bigint CHECK (resume_runtime_ms >= 0),
    resume_at timestamptz,
    watchlist text CHECK (watchlist <> ''),
    added_at timestamptz,
    rating double precision CHECK (rating BETWEEN 0 AND 10),
    seen_at timestamptz NOT NULL,
    CHECK ((kind = 'episode') = (episode > 0)),
    CHECK (kind = 'episode' OR season = 0),
    PRIMARY KEY (user_id, source, entry_key, season, episode)
);

-- The links users, or an administrator for them, made by hand: the
-- entry_key a source names a title by is the movie or series of the IMDb
-- or TMDB identifier, its episodes those of the same season and number. A
-- link applies at once to what import_unfound kept of the entry, and again
-- at each later import of the source that meets it. name and year are
-- those of the title found when it was linked, empty and null when no
-- metadata addon described it. linked_by is who linked it.
CREATE TABLE import_title_links (
    user_id uuid NOT NULL REFERENCES users ON DELETE CASCADE,
    source text NOT NULL CHECK (source ~ '^[a-z]+$'),
    entry_key text NOT NULL CHECK (entry_key <> ''),
    kind text NOT NULL CHECK (kind IN ('movie', 'series')),
    imdb_id text CHECK (imdb_id ~ '^tt[0-9]{1,12}$'),
    tmdb_id integer CHECK (tmdb_id > 0),
    name text NOT NULL DEFAULT '',
    year integer CHECK (year > 0),
    linked_by uuid REFERENCES users ON DELETE SET NULL,
    linked_at timestamptz NOT NULL,
    CHECK (imdb_id IS NOT NULL OR tmdb_id IS NOT NULL),
    PRIMARY KEY (user_id, source, entry_key)
);
