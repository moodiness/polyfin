-- The text subtitles remuxes extracted from a version: the cues of each
-- embedded track, and the spans of the version over which every cue was
-- extracted. Once the spans cover the whole version, its tracks are served
-- whole.
CREATE TABLE media_subtitles (
    version_id uuid PRIMARY KEY,
    extracted jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
