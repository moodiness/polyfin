-- What versions' Matroska indexes say about their text subtitles, and the
-- tracks read whole through them. The Cues of a Matroska file usually list
-- every block of its subtitle tracks: a whole track is then read from small
-- spans around its blocks rather than the whole file, so that apps taking
-- subtitles only as files get the tracks inside files from the start.

-- The FFmpeg indexes of the text subtitle streams a version's index lists
-- block by block, but those whose blocks could not be read; empty when
-- there are none.
CREATE TABLE media_subtitle_index (
    version_id uuid PRIMARY KEY,
    located integer[] NOT NULL,
    indexed_at timestamptz NOT NULL DEFAULT now()
);

-- Text subtitle tracks read whole, each as the file it makes in its own
-- format: srt, vtt or ass.
CREATE TABLE media_subtitle_tracks (
    version_id uuid NOT NULL,
    -- The track's FFmpeg stream index.
    stream integer NOT NULL,
    format text NOT NULL,
    data bytea NOT NULL,
    read_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (version_id, stream)
);
