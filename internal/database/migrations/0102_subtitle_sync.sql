-- Whether the addons' subtitles are timed to the version played, against
-- when the version has speech.
ALTER TABLE settings
    ADD COLUMN sync_addon_subtitles boolean NOT NULL DEFAULT true;

-- When a version has speech, found once, in the background, at its first
-- play: from a text track inside its file ('track'), else from the voice
-- band of its audio ('audio'), else none could be found ('none'). windows
-- are the stretches looked at and speech the spans of speech in them, each
-- a list of [start, end] pairs in milliseconds.
CREATE TABLE version_speech (
    version_id uuid PRIMARY KEY,
    reference text NOT NULL CHECK (reference IN ('track', 'audio', 'none')),
    windows jsonb NOT NULL DEFAULT '[]',
    speech jsonb NOT NULL DEFAULT '[]',
    found_at timestamptz NOT NULL DEFAULT now()
);

-- How each addon subtitle chosen for a version is timed to the version's
-- speech: its times multiplied by ratio, then shifted by offset_ms. A
-- subtitle left as it is has a ratio of 1 and no offset.
CREATE TABLE subtitle_syncs (
    version_id uuid NOT NULL REFERENCES version_speech ON DELETE CASCADE,
    subtitle_id uuid NOT NULL,
    ratio double precision NOT NULL CHECK (ratio > 0),
    offset_ms integer NOT NULL,
    synced_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (version_id, subtitle_id)
);
