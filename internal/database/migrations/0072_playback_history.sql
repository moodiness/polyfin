-- Whether Polyfin keeps a history of the videos played, for the
-- statistics, and for how many days.
ALTER TABLE settings
    ADD COLUMN playback_history boolean NOT NULL DEFAULT true,
    ADD COLUMN playback_history_days integer NOT NULL DEFAULT 365 CHECK (playback_history_days BETWEEN 1 AND 3650);

-- One row per video played: the user, the item and what it was called then
-- (its name, its series, season and episode numbers for an episode, and
-- its channel for a channel, a Replay programme or a recording), the app
-- and device it played on, when it started and ended, how long it played
-- (pauses left out), the last position, both in seconds, and how it
-- reached the app: 'direct_play', 'direct_stream' (remuxed), 'conversion',
-- or '' when no report told.
CREATE TABLE playback_history (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users ON DELETE CASCADE,
    item_id uuid NOT NULL,
    kind text NOT NULL CHECK (kind IN ('movie', 'episode', 'channel', 'recording', 'replay')),
    name text NOT NULL,
    series_id uuid,
    series_name text NOT NULL DEFAULT '',
    season integer,
    episode integer,
    channel_id uuid,
    channel_name text NOT NULL DEFAULT '',
    app text NOT NULL DEFAULT '',
    device text NOT NULL DEFAULT '',
    started_at timestamptz NOT NULL,
    ended_at timestamptz NOT NULL CHECK (ended_at >= started_at),
    played_seconds integer NOT NULL CHECK (played_seconds >= 0),
    position_seconds integer NOT NULL DEFAULT 0 CHECK (position_seconds >= 0),
    method text NOT NULL DEFAULT '' CHECK (method IN ('', 'direct_play', 'direct_stream', 'conversion'))
);

CREATE INDEX playback_history_started ON playback_history (started_at);
CREATE INDEX playback_history_user ON playback_history (user_id, started_at);
