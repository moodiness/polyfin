-- Only the subtitle tracks inside the files are offered; on by default,
-- existing servers included.
ALTER TABLE settings ADD COLUMN in_file_subtitles_only boolean NOT NULL DEFAULT true;
