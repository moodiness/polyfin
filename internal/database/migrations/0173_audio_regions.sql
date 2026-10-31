-- The regional variant of audio a user prefers, a BCP 47 tag such as
-- fr-CA or es-419 among those Polyfin tells apart, empty for none. It
-- refines the audio language of their own configuration or the server's.
ALTER TABLE users ADD COLUMN audio_region text NOT NULL DEFAULT '' CHECK (octet_length(audio_region) <= 16);
