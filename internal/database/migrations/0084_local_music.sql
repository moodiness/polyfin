-- A local folder may hold music, which answers as an Eclipse addon does.
ALTER TABLE local_folders
    DROP CONSTRAINT local_folders_kind_check,
    ADD CONSTRAINT local_folders_kind_check CHECK (kind IN ('movies', 'shows', 'music'));

-- The audio files a scan found in a music folder. path is the file's, in
-- the folder, with slashes; size and modified tell whether it changed
-- since its tags were read. title, artist, album, track, disc, year and
-- genres are what its tags tell, else its file and folder names;
-- album_artist is the artist its album is grouped under: its album artist
-- tag, else "Various Artists" for a compilation, else its artist.
-- artist_id, album_artist_id and album_id identify them by name (see
-- localfiles.nameID). duration is in seconds; codec, container, sample_rate
-- and bit_depth describe its audio; cover tells that it embeds a cover. A
-- file whose tags and names give no title or no album has the reason why,
-- and is left out of the folder's albums.
CREATE TABLE local_tracks (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    addon_id uuid NOT NULL REFERENCES local_folders ON DELETE CASCADE,
    path text NOT NULL CHECK (path <> ''),
    size bigint NOT NULL CHECK (size >= 0),
    modified timestamptz NOT NULL,
    title text NOT NULL DEFAULT '',
    artist text NOT NULL DEFAULT '',
    album text NOT NULL DEFAULT '',
    album_artist text NOT NULL DEFAULT '',
    artist_id text NOT NULL DEFAULT '',
    album_artist_id text NOT NULL DEFAULT '',
    album_id text NOT NULL DEFAULT '',
    track integer NOT NULL DEFAULT 0,
    disc integer NOT NULL DEFAULT 0,
    year integer NOT NULL DEFAULT 0,
    genres text[] NOT NULL DEFAULT '{}',
    duration double precision NOT NULL DEFAULT 0,
    codec text NOT NULL DEFAULT '',
    container text NOT NULL DEFAULT '',
    sample_rate integer NOT NULL DEFAULT 0,
    bit_depth integer NOT NULL DEFAULT 0,
    cover boolean NOT NULL DEFAULT false,
    reason text NOT NULL DEFAULT '' CHECK (reason IN ('', 'no_title', 'no_album')),
    UNIQUE (addon_id, path)
);
CREATE INDEX local_tracks_albums ON local_tracks (addon_id, album_id);

-- The artwork of a music folder's albums: the cover a track embeds (data,
-- of content_type), else an image beside its tracks (path). source is the
-- file it came from, with its size and modification time then, to read it
-- again once it changes.
CREATE TABLE local_covers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    addon_id uuid NOT NULL REFERENCES local_folders ON DELETE CASCADE,
    album_id text NOT NULL CHECK (album_id <> ''),
    path text NOT NULL DEFAULT '',
    data bytea,
    content_type text NOT NULL DEFAULT '',
    source text NOT NULL,
    size bigint NOT NULL,
    modified timestamptz NOT NULL,
    UNIQUE (addon_id, album_id),
    CHECK ((path = '') = (data IS NOT NULL))
);
