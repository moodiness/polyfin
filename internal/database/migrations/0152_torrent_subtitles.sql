-- The text subtitle files inside the torrents of torrent versions, those
-- that go with the video a version plays: listed once its torrent's files
-- are known, at its first play, and kept as the tracks read from files
-- are, so that they stay listed once the torrent stopped. A version listed
-- has its row in torrent_subtitle_lists, with or without files.
CREATE TABLE torrent_subtitle_lists (
    version_id uuid PRIMARY KEY,
    listed_at timestamptz NOT NULL DEFAULT now()
);

-- Each file: file_index is its index among the torrent's files, path its
-- path in the torrent; format is its kind, by its extension; language,
-- title, forced, hearing_impaired and is_default are what its name tells.
-- data is its text as the torrent holds it, NULL until it is fetched.
CREATE TABLE torrent_subtitles (
    version_id uuid NOT NULL REFERENCES torrent_subtitle_lists ON DELETE CASCADE,
    file_index integer NOT NULL CHECK (file_index >= 0),
    path text NOT NULL CHECK (path <> ''),
    format text NOT NULL CHECK (format IN ('srt', 'ass', 'ssa', 'vtt')),
    language text NOT NULL DEFAULT '',
    title text NOT NULL DEFAULT '',
    forced boolean NOT NULL DEFAULT false,
    hearing_impaired boolean NOT NULL DEFAULT false,
    is_default boolean NOT NULL DEFAULT false,
    data bytea,
    fetched_at timestamptz,
    PRIMARY KEY (version_id, file_index)
);
