-- How PlaybackInfo chooses a version and how the server converts video:
-- how long one ffprobe analysis may take, in seconds; how many versions are
-- analyzed when the app chose none; whether versions the app plays without
-- conversion are preferred; how many playbacks may have their video
-- converted at once (0 for no limit); and the height converted video is
-- scaled down to at most (0 keeps the original's). The defaults are what
-- Polyfin did before.
ALTER TABLE settings
    ADD COLUMN analysis_timeout integer NOT NULL DEFAULT 45 CHECK (analysis_timeout BETWEEN 5 AND 120),
    ADD COLUMN version_attempts integer NOT NULL DEFAULT 3 CHECK (version_attempts BETWEEN 1 AND 10),
    ADD COLUMN prefer_direct_play boolean NOT NULL DEFAULT false,
    ADD COLUMN max_conversions integer NOT NULL DEFAULT 0 CHECK (max_conversions BETWEEN 0 AND 32),
    ADD COLUMN max_conversion_height integer NOT NULL DEFAULT 0 CHECK (max_conversion_height IN (0, 480, 720, 1080, 1440, 2160));
