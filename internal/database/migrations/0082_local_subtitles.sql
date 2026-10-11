-- The subtitle files a scan found beside the video files of a local
-- folder: path is the file's, in the folder, with slashes; file_id the
-- video it belongs to, whose name its own begins with; size and modified
-- tell whether it changed. format is its kind, by its extension; language,
-- title, forced, hearing_impaired and is_default are what its name tells
-- past the video's.
CREATE TABLE local_subtitles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    addon_id uuid NOT NULL REFERENCES local_folders ON DELETE CASCADE,
    file_id uuid NOT NULL REFERENCES local_files ON DELETE CASCADE,
    path text NOT NULL CHECK (path <> ''),
    size bigint NOT NULL CHECK (size >= 0),
    modified timestamptz NOT NULL,
    format text NOT NULL CHECK (format IN ('srt', 'ass', 'ssa', 'vtt')),
    language text NOT NULL DEFAULT '',
    title text NOT NULL DEFAULT '',
    forced boolean NOT NULL DEFAULT false,
    hearing_impaired boolean NOT NULL DEFAULT false,
    is_default boolean NOT NULL DEFAULT false,
    UNIQUE (addon_id, path)
);
CREATE INDEX local_subtitles_files ON local_subtitles (file_id);
