-- Converted video can be kept apart from its audio, so that an app changing
-- the audio track gets the video already converted. Off by default: off,
-- conversions work as they did.
ALTER TABLE settings
    ADD COLUMN keep_converted_video boolean NOT NULL DEFAULT false;
