-- XMLTV guides mark a programme new (<new/>) or a repeat
-- (<previously-shown>); one marked new is not kept as a repeat.
ALTER TABLE live_guide_programmes
    ADD COLUMN is_new boolean NOT NULL DEFAULT false,
    ADD COLUMN previously_shown boolean NOT NULL DEFAULT false;

-- The episode a timer's programme airs, and so its recording: its season
-- and episode numbers, 0 when the guide does not number it, and its
-- episode title.
ALTER TABLE live_timers
    ADD COLUMN season integer NOT NULL DEFAULT 0 CHECK (season >= 0),
    ADD COLUMN episode integer NOT NULL DEFAULT 0 CHECK (episode >= 0),
    ADD COLUMN episode_title text NOT NULL DEFAULT '';
ALTER TABLE live_recordings
    ADD COLUMN season integer NOT NULL DEFAULT 0 CHECK (season >= 0),
    ADD COLUMN episode integer NOT NULL DEFAULT 0 CHECK (episode >= 0),
    ADD COLUMN episode_title text NOT NULL DEFAULT '';

-- The episodes a series timer recorded in full, kept once their recordings
-- are deleted: "new episodes only" does not record them again. An episode
-- is known by its numbers, else its title, else its description.
CREATE TABLE live_series_episodes (
    series_timer_id uuid NOT NULL REFERENCES live_series_timers (id) ON DELETE CASCADE,
    season integer NOT NULL DEFAULT 0 CHECK (season >= 0),
    episode integer NOT NULL DEFAULT 0 CHECK (episode >= 0),
    title text NOT NULL DEFAULT '',
    description text NOT NULL DEFAULT '',
    recorded_at timestamptz NOT NULL DEFAULT now(),
    CHECK (episode > 0 OR title <> '' OR description <> '')
);
CREATE INDEX live_series_episodes_series ON live_series_episodes (series_timer_id);
