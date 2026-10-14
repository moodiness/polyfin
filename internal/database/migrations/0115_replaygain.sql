-- Even the volume between songs, on by default: whether apps are given
-- how much to raise or lower each song and album. measure_loudness tells
-- which songs without a gain have their loudness measured: none, those of
-- local music folders, or those of music addons too.
ALTER TABLE settings
    ADD COLUMN even_volume boolean NOT NULL DEFAULT true,
    ADD COLUMN measure_loudness text NOT NULL DEFAULT 'off' CHECK (measure_loudness IN ('off', 'local', 'all'));

-- The gains of the tracks of local music folders, in dB relative to
-- -18 LUFS: track_gain and album_gain are their ReplayGain tags', or
-- their Opus R128 tags' moved to that reference, null without one.
-- loudness is the track's integrated loudness as measured, in LUFS, null
-- until measured; a measure that failed counts in measure_failures, and
-- is tried again from measure_after. A file that changes is read and
-- measured again. gains_read is false for the tracks read before gains
-- were, which the next scan reads again, once.
ALTER TABLE local_tracks
    ADD COLUMN track_gain double precision,
    ADD COLUMN album_gain double precision,
    ADD COLUMN loudness double precision,
    ADD COLUMN measure_failures integer NOT NULL DEFAULT 0,
    ADD COLUMN measure_after timestamptz,
    ADD COLUMN gains_read boolean NOT NULL DEFAULT false;
ALTER TABLE local_tracks ALTER COLUMN gains_read SET DEFAULT true;

-- The loudness of the songs of music addons, measured after their first
-- play, by the song's item: album_id is its album's item, null for none,
-- and length its length in seconds, which weighs it in its album's
-- loudness. loudness is in LUFS, null while the song could not be
-- measured: failures counts the attempts that failed, the next one not
-- before retry_at.
CREATE TABLE track_loudness (
    item_id uuid PRIMARY KEY,
    album_id uuid,
    length double precision NOT NULL DEFAULT 0,
    loudness double precision,
    failures integer NOT NULL DEFAULT 0,
    retry_at timestamptz,
    measured_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX track_loudness_albums ON track_loudness (album_id) WHERE album_id IS NOT NULL;
