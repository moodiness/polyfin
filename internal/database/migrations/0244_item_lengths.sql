-- A movie's runtime as TMDB's details give it, in minutes: 0 when they
-- give none, and for a series, whose page shows none. NULL until it is
-- read, which makes the row due: the rows kept before are read again
-- once.
ALTER TABLE title_credits
    ADD COLUMN runtime integer CHECK (runtime >= 0);

-- The lengths TMDB's seasons give the episodes of a series, by the series'
-- Stremio identifier, for its episodes whose description gives none:
-- minutes by the addon's season and number ("2:8"), as the series'
-- numbering pairs them with TMDB's. tmdb_id is NULL for a series TMDB
-- does not know. Read again once asked_at is 30 days old.
CREATE TABLE tmdb_episode_lengths (
    series text PRIMARY KEY,
    tmdb_id integer CHECK (tmdb_id > 0),
    lengths jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(lengths) = 'object'),
    asked_at timestamptz NOT NULL
);

-- How long a video of a movies or shows folder plays, in seconds, as
-- ffprobe read it at the scan that found it: 0 when it could not, NULL
-- until it is probed, and again once the file changes. A disc's title
-- has its length instead.
ALTER TABLE local_files
    ADD COLUMN duration double precision CHECK (duration >= 0);
