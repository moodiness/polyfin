-- The side files a scan read beside the video files of a local folder: the
-- NFO files that may tell the title of a unit, a movie's, a disc's or a
-- show's (see local_files). path is the file's, in the folder, with
-- slashes; size and modified tell whether it changed since it was read.
-- imdb_id and tmdb_id are the identifiers it gives, empty when it gives
-- none.
CREATE TABLE local_side_files (
    addon_id uuid NOT NULL REFERENCES local_folders ON DELETE CASCADE,
    path text NOT NULL CHECK (path <> ''),
    size bigint NOT NULL CHECK (size >= 0),
    modified timestamptz NOT NULL,
    imdb_id text NOT NULL DEFAULT '',
    tmdb_id text NOT NULL DEFAULT '',
    PRIMARY KEY (addon_id, path)
);

-- nfo_id is the identifier the NFO file of a file's unit gave when the
-- unit was last read, an IMDb identifier or "tmdb:…", empty for none: a
-- change matches the unit again. A file may be matched by it.
ALTER TABLE local_files
    ADD COLUMN nfo_id text NOT NULL DEFAULT '',
    DROP CONSTRAINT local_files_matched_by_check,
    ADD CONSTRAINT local_files_matched_by_check CHECK (matched_by IN ('', 'name', 'nfo', 'search', 'link'));
