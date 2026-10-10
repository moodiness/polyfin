-- When the weekly summary is sent: the day of the week, 0 for Sunday to 6
-- for Saturday, and the hour, both in the server's time zone. Monday at
-- 9:00 by default.
ALTER TABLE settings
    ADD COLUMN weekly_summary_day integer NOT NULL DEFAULT 1 CHECK (weekly_summary_day BETWEEN 0 AND 6),
    ADD COLUMN weekly_summary_hour integer NOT NULL DEFAULT 9 CHECK (weekly_summary_hour BETWEEN 0 AND 23);

-- The end of the last week the summary was sent for, its scheduled day and
-- hour: one row. A restart does not send the same week again, and a server
-- that was off at the hour sends it when it starts again. It starts at the
-- migration, so that no week before it is sent.
CREATE TABLE notification_summary (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    week_end timestamptz NOT NULL
);
INSERT INTO notification_summary (week_end) VALUES (now());

-- The new episodes the new-episode check found for each user, with what
-- they were called then, for the weekly summary: an episode that was
-- available when its series was first seen is not one.
CREATE TABLE notification_found_episodes (
    user_id uuid NOT NULL REFERENCES users ON DELETE CASCADE,
    episode_id uuid NOT NULL,
    series_id uuid NOT NULL,
    series_name text NOT NULL,
    name text NOT NULL,
    season integer NOT NULL,
    number integer NOT NULL,
    found_at timestamptz NOT NULL,
    PRIMARY KEY (user_id, episode_id)
);

CREATE INDEX notification_found_episodes_found ON notification_found_episodes (found_at);

-- The users who joined through an invite link, and through which, for the
-- weekly summary.
CREATE TABLE notification_joins (
    user_id uuid PRIMARY KEY REFERENCES users ON DELETE CASCADE,
    invite_id uuid NOT NULL,
    joined_at timestamptz NOT NULL
);

-- The problems the health check told, when it told them, with the page of
-- the admin app that shows each, for the weekly summary.
CREATE TABLE notification_health_found (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    key text NOT NULL,
    severity text NOT NULL CHECK (severity IN ('error', 'warning')),
    text text NOT NULL,
    page text NOT NULL DEFAULT '',
    found_at timestamptz NOT NULL
);

CREATE INDEX notification_health_found_found ON notification_health_found (found_at);

-- When a scan first found each file of a local folder, for the weekly
-- summary; null for the files found before this column existed.
ALTER TABLE local_files ADD COLUMN added_at timestamptz;
ALTER TABLE local_files ALTER COLUMN added_at SET DEFAULT now();
CREATE INDEX local_files_added ON local_files (added_at) WHERE added_at IS NOT NULL;
