-- Version names end with how the app asking plays each version, as its
-- last PlaybackInfo's device profile decides it: on by default.
ALTER TABLE settings
    ADD COLUMN name_how_versions_play boolean NOT NULL DEFAULT true;
