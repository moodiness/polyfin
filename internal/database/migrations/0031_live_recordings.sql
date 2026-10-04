-- Live TV recording. A user may schedule and delete recordings with
-- Jellyfin's EnableLiveTvManagement; administrators could already reach
-- every Live TV route, so they keep it.
ALTER TABLE users ADD COLUMN live_tv_management boolean NOT NULL DEFAULT false;
UPDATE users SET live_tv_management = is_administrator;

-- How long new recordings start before their programme and go on after it,
-- in seconds, Jellyfin's defaults being none, and after how many days a
-- recording is deleted, 0 for never.
ALTER TABLE settings
    ADD COLUMN recording_pre_padding integer NOT NULL DEFAULT 0 CHECK (recording_pre_padding BETWEEN 0 AND 3600),
    ADD COLUMN recording_post_padding integer NOT NULL DEFAULT 0 CHECK (recording_post_padding BETWEEN 0 AND 3600),
    ADD COLUMN recording_retention_days integer NOT NULL DEFAULT 0 CHECK (recording_retention_days BETWEEN 0 AND 3650);

-- Series timers record every upcoming programme of the same title, on the
-- channel and at the time of day of the programme they were made from,
-- unless they record on any channel or at any time. The user who made one
-- is the one whose channels its recordings are read from.
CREATE TABLE live_series_timers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    channel_id uuid NOT NULL,
    program_id uuid,
    name text NOT NULL CHECK (name <> ''),
    overview text NOT NULL DEFAULT '',
    start_at timestamptz NOT NULL,
    end_at timestamptz NOT NULL,
    record_any_channel boolean NOT NULL DEFAULT false,
    record_any_time boolean NOT NULL DEFAULT true,
    record_new_only boolean NOT NULL DEFAULT true,
    skip_episodes_in_library boolean NOT NULL DEFAULT true,
    days text[] NOT NULL DEFAULT '{}' CHECK (days <@ ARRAY['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']),
    keep_up_to integer NOT NULL DEFAULT 0 CHECK (keep_up_to BETWEEN 0 AND 1000),
    keep_until text NOT NULL DEFAULT 'UntilDeleted'
        CHECK (keep_until IN ('UntilDeleted', 'UntilSpaceNeeded', 'UntilWatched', 'UntilDate')),
    priority integer NOT NULL DEFAULT 0,
    pre_padding integer NOT NULL DEFAULT 0 CHECK (pre_padding BETWEEN 0 AND 3600),
    post_padding integer NOT NULL DEFAULT 0 CHECK (post_padding BETWEEN 0 AND 3600),
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Timers record one programme, from start_at less pre_padding seconds to
-- end_at plus post_padding, as Jellyfin's do; a programme has one timer at
-- most. The programme's description is kept for the recording.
CREATE TABLE live_timers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    series_timer_id uuid REFERENCES live_series_timers (id) ON DELETE CASCADE,
    channel_id uuid NOT NULL,
    program_id uuid UNIQUE,
    name text NOT NULL CHECK (name <> ''),
    overview text NOT NULL DEFAULT '',
    genres text[] NOT NULL DEFAULT '{}',
    rating text NOT NULL DEFAULT '',
    image text NOT NULL DEFAULT '',
    start_at timestamptz NOT NULL,
    end_at timestamptz NOT NULL CHECK (end_at > start_at),
    pre_padding integer NOT NULL DEFAULT 0 CHECK (pre_padding BETWEEN 0 AND 3600),
    post_padding integer NOT NULL DEFAULT 0 CHECK (post_padding BETWEEN 0 AND 3600),
    keep_until text NOT NULL DEFAULT 'UntilDeleted'
        CHECK (keep_until IN ('UntilDeleted', 'UntilSpaceNeeded', 'UntilWatched', 'UntilDate')),
    priority integer NOT NULL DEFAULT 0,
    status text NOT NULL DEFAULT 'New' CHECK (status IN ('New', 'InProgress', 'Completed', 'Cancelled', 'Error')),
    -- A timer made or kept by hand stays when its series timer changes.
    is_manual boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX live_timers_series ON live_timers (series_timer_id);

-- Recordings are files of the recordings folder, named after their id. One
-- in progress is still being written; one stopped before its end, by a
-- restart or a source that failed, is partial.
CREATE TABLE live_recordings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    timer_id uuid,
    series_timer_id uuid,
    user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    channel_id uuid NOT NULL,
    program_id uuid,
    name text NOT NULL CHECK (name <> ''),
    overview text NOT NULL DEFAULT '',
    genres text[] NOT NULL DEFAULT '{}',
    rating text NOT NULL DEFAULT '',
    image text NOT NULL DEFAULT '',
    start_at timestamptz NOT NULL,
    end_at timestamptz NOT NULL,
    started_at timestamptz NOT NULL DEFAULT now(),
    ended_at timestamptz,
    status text NOT NULL DEFAULT 'InProgress' CHECK (status IN ('InProgress', 'Completed')),
    partial boolean NOT NULL DEFAULT false,
    file text NOT NULL DEFAULT '',
    size bigint NOT NULL DEFAULT 0 CHECK (size >= 0),
    CHECK ((status = 'InProgress') = (ended_at IS NULL))
);
CREATE INDEX live_recordings_started ON live_recordings (started_at DESC);
