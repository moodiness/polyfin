-- The regional variant of the audio track a user chose on a title, one of
-- localization.AudioVariants ("fr-FR"), kept only when it differs from
-- the user's audio region: among the tracks in audio_language, the next
-- plays prefer it to the region's. Empty when the track's variant is the
-- user's region, unknown, or the user has no region in its language.
ALTER TABLE track_choices
    ADD COLUMN audio_variant text NOT NULL DEFAULT '' CHECK (octet_length(audio_variant) <= 35),
    ADD CHECK (audio_variant = '' OR audio_language <> '');
