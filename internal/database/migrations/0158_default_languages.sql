-- The audio and subtitle languages of the users who chose none, ISO 639-2/B
-- codes as their own preferences take them: empty, the default, for none.
ALTER TABLE settings
    ADD COLUMN default_audio_language text NOT NULL DEFAULT '' CHECK (length(default_audio_language) <= 3),
    ADD COLUMN default_subtitle_language text NOT NULL DEFAULT '' CHECK (length(default_subtitle_language) <= 3);
