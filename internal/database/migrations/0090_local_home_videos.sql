-- A local folder may hold home videos: each of its videos is a title of
-- its own, which no metadata addon knows.
ALTER TABLE local_folders
    DROP CONSTRAINT local_folders_kind_check,
    ADD CONSTRAINT local_folders_kind_check CHECK (kind IN ('movies', 'shows', 'music', 'homevideos'));

-- A home video's file is the title of its own identifier, matched_by
-- "own" (see localfiles.videoID).
ALTER TABLE local_files
    DROP CONSTRAINT local_files_matched_by_check,
    ADD CONSTRAINT local_files_matched_by_check CHECK (matched_by IN ('', 'name', 'nfo', 'search', 'link', 'own'));

-- A home video's playbacks are kept as the history keeps movies'.
ALTER TABLE playback_history
    DROP CONSTRAINT playback_history_kind_check,
    ADD CONSTRAINT playback_history_kind_check CHECK (kind IN ('movie', 'episode', 'channel', 'recording', 'replay', 'video'));

-- The title, plot, date and year an NFO file gives, which home videos
-- show: empty, null or 0 when it gives none.
ALTER TABLE local_side_files
    ADD COLUMN title text NOT NULL DEFAULT '',
    ADD COLUMN plot text NOT NULL DEFAULT '',
    ADD COLUMN released date,
    ADD COLUMN year integer NOT NULL DEFAULT 0;

-- What a scan read of each video of a home videos folder, beside its
-- local_files row. size and modified are the file's when it was probed:
-- created is the creation_time its container tells, null for none,
-- duration its length in seconds, 0 when unknown, and hdr whether its
-- video is HDR. released is the date the title shows, and dated_by where
-- it comes from: its NFO file ("nfo"), created ("created") or modified
-- ("file"); plot is its NFO file's. frame is a JPEG of a frame of the
-- video, which the title shows without a poster beside it, opened by
-- frame_id (see localfiles.Open); framed tells that it was made, or
-- tried, for the file as it is.
CREATE TABLE local_videos (
    addon_id uuid NOT NULL,
    path text NOT NULL,
    size bigint NOT NULL CHECK (size >= 0),
    modified timestamptz NOT NULL,
    created timestamptz,
    duration double precision NOT NULL DEFAULT 0,
    hdr boolean NOT NULL DEFAULT false,
    released timestamptz NOT NULL,
    dated_by text NOT NULL CHECK (dated_by IN ('nfo', 'created', 'file')),
    plot text NOT NULL DEFAULT '',
    frame bytea,
    frame_id uuid NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    framed boolean NOT NULL DEFAULT false,
    PRIMARY KEY (addon_id, path),
    FOREIGN KEY (addon_id, path) REFERENCES local_files (addon_id, path) ON DELETE CASCADE
);
