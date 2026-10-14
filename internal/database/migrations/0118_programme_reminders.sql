-- How long before a programme a user's reminders come, in minutes, and
-- whether they also show on the user's Jellyfin apps that accept messages.
-- A user without a row has the defaults.
CREATE TABLE reminder_settings (
    user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    before_minutes integer NOT NULL DEFAULT 5 CHECK (before_minutes IN (1, 5, 10, 15, 30, 60)),
    on_apps boolean NOT NULL DEFAULT false
);

-- Series reminders remind a user of every airing of a title on a channel
-- at the time of day of the programme they were made from, within ten
-- minutes, as series timers find them. language is the web client's the
-- reminder was set in, which its messages are written in.
CREATE TABLE programme_series_reminders (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    channel_id uuid NOT NULL,
    channel_name text NOT NULL DEFAULT '',
    name text NOT NULL CHECK (name <> ''),
    start_at timestamptz NOT NULL,
    language text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX programme_series_reminders_user ON programme_series_reminders (user_id);

-- A reminder tells its user, once, that a programme starts soon: one set
-- by hand, or one of a series reminder's airings. start_at follows the
-- guide as the programme moves; handled_at is when it was sent, or given
-- up because its programme was gone, had started or was no longer the
-- user's to see, and sent tells which. A restart finds it handled and
-- sends it no more.
CREATE TABLE programme_reminders (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    series_id uuid REFERENCES programme_series_reminders (id) ON DELETE CASCADE,
    program_id uuid NOT NULL,
    channel_id uuid NOT NULL,
    channel_name text NOT NULL DEFAULT '',
    name text NOT NULL CHECK (name <> ''),
    genres text[] NOT NULL DEFAULT '{}',
    rating text NOT NULL DEFAULT '',
    start_at timestamptz NOT NULL,
    end_at timestamptz NOT NULL CHECK (end_at > start_at),
    language text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    handled_at timestamptz,
    sent boolean NOT NULL DEFAULT false,
    UNIQUE (user_id, program_id)
);
CREATE INDEX programme_reminders_pending ON programme_reminders (start_at) WHERE handled_at IS NULL;
CREATE INDEX programme_reminders_series ON programme_reminders (series_id);
