-- Whether the scans of a movies or shows folder read the extras, local
-- trailers and theme media beside its titles (see local_extras).
ALTER TABLE local_folders
    ADD COLUMN read_extras boolean NOT NULL DEFAULT true;

-- The extras a scan found beside the titles of a movies or shows folder,
-- named as Jellyfin names them: in a folder such as "Extras" or
-- "Trailers", with a suffix such as "-trailer", or a theme song or video.
-- id is the extra's item (see library.ExtraItemID), path its file's, in
-- the folder, with slashes; size and modified tell whether it changed.
-- extra_type is Jellyfin's ExtraType, name what apps show. The extra is
-- one of the titles the files of units are matched to (see local_files):
-- a movie's file or disc, or a show's folder, with season one of the
-- show's seasons.
CREATE TABLE local_extras (
    id uuid PRIMARY KEY,
    addon_id uuid NOT NULL REFERENCES local_folders ON DELETE CASCADE,
    path text NOT NULL CHECK (path <> ''),
    size bigint NOT NULL CHECK (size >= 0),
    modified timestamptz NOT NULL,
    extra_type text NOT NULL CHECK (extra_type IN ('Trailer', 'BehindTheScenes', 'DeletedScene', 'Interview', 'Scene', 'Sample',
        'Short', 'Featurette', 'Clip', 'Unknown', 'ThemeSong', 'ThemeVideo')),
    name text NOT NULL,
    units text[] NOT NULL CHECK (cardinality(units) > 0),
    season integer CHECK (season >= 0),
    UNIQUE (addon_id, path)
);

-- The subtitle files beside an extra are its own, as those beside a
-- video are the video's.
ALTER TABLE local_subtitles
    ALTER COLUMN file_id DROP NOT NULL,
    ADD COLUMN extra_id uuid REFERENCES local_extras ON DELETE CASCADE,
    ADD CONSTRAINT local_subtitles_one_video CHECK ((file_id IS NULL) <> (extra_id IS NULL));
CREATE INDEX local_subtitles_extras ON local_subtitles (extra_id);
