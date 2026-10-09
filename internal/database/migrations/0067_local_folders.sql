-- How many hours after its last scan a local folder is scanned again, 0
-- for never on a schedule: folders are still scanned at startup and when an
-- administrator asks.
ALTER TABLE settings
    ADD COLUMN local_scan_hours integer NOT NULL DEFAULT 6 CHECK (local_scan_hours BETWEEN 0 AND 168);

-- An addon may be one of Polyfin's own local folders: a folder mounted in
-- the container, whose path is the manifest URL, and whose manifest
-- describes the one catalog of the titles its files were matched to.
ALTER TABLE addons
    DROP CONSTRAINT addons_kind_check,
    ADD CONSTRAINT addons_kind_check CHECK (kind IN ('stremio', 'm3u', 'xtream', 'eclipse', 'local'));

-- A local folder: whether it holds movies or shows, and how its last scan
-- went. checked_at is the last attempt, scanned_at the last scan that could
-- read the folder; error is the code of the last failure, empty after a
-- success.
CREATE TABLE local_folders (
    addon_id uuid PRIMARY KEY REFERENCES addons (id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('movies', 'shows')),
    checked_at timestamptz,
    scanned_at timestamptz,
    error text NOT NULL DEFAULT '' CHECK (error IN ('', 'missing', 'unreadable', 'not_folder'))
);

-- The video files a scan found in a local folder. path is the file's, in
-- the folder, with slashes; size and modified tell whether it changed since.
-- unit is what is matched to a title: the file itself, or, in a shows
-- folder, the show's folder (the first folder of its path). title, year,
-- season, episode, last_episode (the last episode of a file holding
-- several) and height are what its name tells. A matched file has the
-- Stremio identifier of its title (an IMDb identifier, or "tmdb:…"), with
-- the IMDb and TMDB identifiers found, the title's name, poster and year,
-- and how it was matched; an unmatched one has the reason why.
CREATE TABLE local_files (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    addon_id uuid NOT NULL REFERENCES local_folders ON DELETE CASCADE,
    path text NOT NULL CHECK (path <> ''),
    size bigint NOT NULL CHECK (size >= 0),
    modified timestamptz NOT NULL,
    unit text NOT NULL CHECK (unit <> ''),
    title text NOT NULL DEFAULT '',
    year integer,
    season integer CHECK (season >= 0),
    episode integer CHECK (episode >= 0),
    last_episode integer CHECK (last_episode >= episode),
    height integer,
    stremio_id text CHECK (stremio_id <> ''),
    imdb_id text NOT NULL DEFAULT '',
    tmdb_id text NOT NULL DEFAULT '',
    name text NOT NULL DEFAULT '',
    poster text NOT NULL DEFAULT '',
    title_year integer,
    matched_by text NOT NULL DEFAULT '' CHECK (matched_by IN ('', 'name', 'search', 'link')),
    reason text NOT NULL DEFAULT ''
        CHECK (reason IN ('', 'unreadable_name', 'not_found', 'ambiguous', 'other_year', 'search_failed')),
    CHECK ((stremio_id IS NULL) = (matched_by = '')),
    UNIQUE (addon_id, path)
);
CREATE INDEX local_files_titles ON local_files (stremio_id) WHERE stremio_id IS NOT NULL;
CREATE INDEX local_files_units ON local_files (addon_id, unit);

-- The links administrators made by hand: a unit of a local folder (see
-- local_files) is the title of the IMDb identifier, whatever its name says,
-- through every scan.
CREATE TABLE local_links (
    addon_id uuid NOT NULL REFERENCES local_folders ON DELETE CASCADE,
    unit text NOT NULL CHECK (unit <> ''),
    imdb_id text NOT NULL CHECK (imdb_id ~ '^tt[0-9]{1,12}$'),
    PRIMARY KEY (addon_id, unit)
);
