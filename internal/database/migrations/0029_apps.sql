-- What Jellyfin apps keep for a user and a title beyond browsing and
-- playback:
-- - whether the user may add subtitle files to titles, Jellyfin's
--   EnableSubtitleManagement: administrators may, other users may not;
-- - the user's profile picture, normalized to a JPEG or PNG image of at
--   most 5 MB, and its tag, which changes with it ('' without a picture);
-- - the subtitle files users added to movies and episodes, for every
--   version of the title, in a format Polyfin reads.
ALTER TABLE users
    ADD COLUMN subtitle_management boolean NOT NULL DEFAULT false,
    ADD COLUMN image bytea CHECK (octet_length(image) BETWEEN 1 AND 5242880),
    ADD COLUMN image_type text CHECK (image_type IN ('image/jpeg', 'image/png')),
    ADD COLUMN image_tag text NOT NULL DEFAULT '' CHECK (image_tag ~ '^([0-9a-f]{32})?$'),
    ADD CONSTRAINT users_image_complete CHECK ((image IS NULL) = (image_type IS NULL) AND (image IS NULL) = (image_tag = ''));
UPDATE users SET subtitle_management = true WHERE is_administrator;

CREATE TABLE uploaded_subtitles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    item_id uuid NOT NULL,
    language text NOT NULL CHECK (length(language) BETWEEN 1 AND 32),
    format text NOT NULL CHECK (format IN ('srt', 'vtt', 'ass', 'ssa')),
    forced boolean NOT NULL DEFAULT false,
    hearing_impaired boolean NOT NULL DEFAULT false,
    data bytea NOT NULL CHECK (octet_length(data) BETWEEN 1 AND 8388608),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX uploaded_subtitles_item ON uploaded_subtitles (item_id, created_at, id);
