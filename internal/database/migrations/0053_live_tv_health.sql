-- A list or guide whose download failed is tried again after a backoff
-- (5 minutes, 15 minutes, then every hour, never later than
-- LiveTvRefreshHours) rather than once LiveTvRefreshHours have passed:
-- failures counts the failures in a row and next_try_at is the next try,
-- both cleared by a success. A provider that asked to slow down is
-- rate_limited.
ALTER TABLE iptv_sources
    ADD COLUMN failures integer NOT NULL DEFAULT 0 CHECK (failures >= 0),
    ADD COLUMN next_try_at timestamptz,
    -- How many streams an Xtream account may play at once, as its server
    -- says at login; NULL when it does not say, 0 for no limit.
    ADD COLUMN max_connections integer CHECK (max_connections >= 0),
    DROP CONSTRAINT iptv_sources_error_check,
    ADD CONSTRAINT iptv_sources_error_check
        CHECK (error IN ('', 'unreachable', 'private_network', 'too_large', 'malformed', 'rate_limited'));

ALTER TABLE live_guides
    ADD COLUMN failures integer NOT NULL DEFAULT 0 CHECK (failures >= 0),
    ADD COLUMN next_try_at timestamptz,
    DROP CONSTRAINT live_guides_error_check,
    ADD CONSTRAINT live_guides_error_check
        CHECK (error IN ('', 'unreachable', 'private_network', 'too_large', 'malformed', 'channels_unreachable', 'rate_limited'));

-- How a stream of a line-up last answered when a channel was opened:
-- health_url is a hash of the address it was checked at, so that a list
-- giving the stream a new address forgets it; ok_at is the last time it
-- played; failed_at and failure the last failure: dead (an answer that is
-- not a stream), refused (the provider would not serve it now) or timeout
-- (nothing came); failures counts them in a row. A dead stream is left
-- out of its channel for an hour, then six, then a day.
ALTER TABLE iptv_streams
    ADD COLUMN health_url text,
    ADD COLUMN ok_at timestamptz,
    ADD COLUMN failed_at timestamptz,
    ADD COLUMN failure text CHECK (failure IN ('dead', 'refused', 'timeout')),
    ADD COLUMN failures integer NOT NULL DEFAULT 0 CHECK (failures >= 0);

-- What a channel's stream holds (its container and tracks), as ffprobe
-- found it, so that the next start needs no full analysis.
CREATE TABLE live_stream_shapes (
    version_id uuid PRIMARY KEY,
    analysis jsonb NOT NULL CHECK (jsonb_typeof(analysis) = 'object'),
    analyzed_at timestamptz NOT NULL DEFAULT now()
);

-- Programmes of XMLTV guides are no longer kept as items: they are read
-- from the guide when asked for. Those a timer or a recording names are
-- kept for now; the guide refresh deletes them once past.
DELETE FROM items WHERE kind = 'program' AND id NOT IN (
    SELECT program_id FROM live_timers WHERE program_id IS NOT NULL
    UNION SELECT program_id FROM live_series_timers WHERE program_id IS NOT NULL
    UNION SELECT program_id FROM live_recordings WHERE program_id IS NOT NULL);
