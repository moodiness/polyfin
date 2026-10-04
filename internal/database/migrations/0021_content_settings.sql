-- Content settings of the admin interface: whether apps get skip buttons
-- and similar titles, the thresholds that mark a title played and keep a
-- resume point (in percent of its runtime, Jellyfin's defaults), and how
-- long, in minutes, version lists and catalog pages from addons are kept.
ALTER TABLE settings
    ADD COLUMN skip_buttons boolean NOT NULL DEFAULT true,
    ADD COLUMN similar_titles boolean NOT NULL DEFAULT true,
    ADD COLUMN played_percent integer NOT NULL DEFAULT 90 CHECK (played_percent BETWEEN 50 AND 100),
    ADD COLUMN resume_percent integer NOT NULL DEFAULT 5 CHECK (resume_percent BETWEEN 0 AND 50),
    ADD COLUMN version_list_minutes integer NOT NULL DEFAULT 10 CHECK (version_list_minutes BETWEEN 1 AND 360),
    ADD COLUMN catalog_refresh_minutes integer NOT NULL DEFAULT 10 CHECK (catalog_refresh_minutes BETWEEN 1 AND 1440),
    ADD CONSTRAINT settings_resume_below_played CHECK (resume_percent < played_percent);
