-- Two playback switches of the admin interface: whether apps are sent the
-- chapters Polyfin reads when it analyzes a version, and whether Polyfin
-- prepares playback ahead of time, from a title's details and near the end
-- of an episode.
ALTER TABLE settings
    ADD COLUMN chapters boolean NOT NULL DEFAULT true,
    ADD COLUMN prepare_ahead boolean NOT NULL DEFAULT false;
