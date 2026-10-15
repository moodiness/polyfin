-- Picture subtitles read into text with Tesseract: off by default, when a
-- version is played by an app that cannot draw them, or also after the
-- scans of local folders; and how many tracks are read at once.
ALTER TABLE settings
    ADD COLUMN subtitle_ocr text NOT NULL DEFAULT 'off' CHECK (subtitle_ocr IN ('off', 'played', 'scans')),
    ADD COLUMN subtitle_ocr_tracks integer NOT NULL DEFAULT 1 CHECK (subtitle_ocr_tracks BETWEEN 1 AND 4);

-- The text read from each picture subtitle track, kept by what Polyfin
-- knows its file by, as seek_lookbacks are: a track inside a local file by
-- its folder and path (kind 'local'), a track of an addon's file by its
-- version (kind 'version'), a picture file beside a local file by its
-- folder and path (kind 'file', stream -1). file is a local file's size
-- and modification time: the track of a file that changed is read again.
-- text is SubRip, empty when the pictures held no text.
CREATE TABLE subtitle_ocr (
    kind text NOT NULL CHECK (kind IN ('local', 'version', 'file')),
    name text NOT NULL CHECK (name <> ''),
    stream integer NOT NULL CHECK (stream >= -1),
    file text NOT NULL DEFAULT '',
    language text NOT NULL,
    text text NOT NULL,
    read_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (kind, name, stream)
);
