-- A music folder's tracks name their artists and album artists one by
-- one: artists are the names of the artists tag, else of the artist tag,
-- split at ";"; album_artists those of the album artist tag, else the one
-- name album_artist is. artist_ids and album_artist_ids identify them by
-- name (see localfiles.nameID). lyrics is a track's lyrics tag. added_at
-- is when a scan first found the track, for the weekly summary; null for
-- the tracks found before. The tracks found before are read again by the
-- next scan, for their artists and lyrics tags.
ALTER TABLE local_tracks
    ADD COLUMN artists text[] NOT NULL DEFAULT '{}',
    ADD COLUMN artist_ids text[] NOT NULL DEFAULT '{}',
    ADD COLUMN album_artists text[] NOT NULL DEFAULT '{}',
    ADD COLUMN album_artist_ids text[] NOT NULL DEFAULT '{}',
    ADD COLUMN lyrics text NOT NULL DEFAULT '',
    ADD COLUMN added_at timestamptz;
UPDATE local_tracks SET artists = ARRAY[artist], artist_ids = ARRAY[artist_id], album_artists = ARRAY[album_artist],
    album_artist_ids = ARRAY[album_artist_id], modified = 'epoch';
ALTER TABLE local_tracks
    DROP COLUMN artist_id,
    DROP COLUMN album_artist_id,
    ALTER COLUMN added_at SET DEFAULT now();
CREATE INDEX local_tracks_added ON local_tracks (added_at) WHERE added_at IS NOT NULL;
-- Playlists and lyrics files name tracks by path, in any letter case.
CREATE INDEX local_tracks_paths ON local_tracks (addon_id, lower(path));

-- local_covers holds the pictures of a music folder's artists too: item_id
-- is an album's identifier, or an artist's (see localfiles.nameID).
ALTER TABLE local_covers RENAME COLUMN album_id TO item_id;
ALTER TABLE local_covers RENAME CONSTRAINT local_covers_album_id_check TO local_covers_item_id_check;
ALTER TABLE local_covers RENAME CONSTRAINT local_covers_addon_id_album_id_key TO local_covers_addon_id_item_id_key;

-- The lyrics files of a music folder (.lrc), each the lyrics of the
-- tracks of the same name beside it: text is what it holds, read when it
-- was found or changed, by its size and modification time.
CREATE TABLE local_lyrics (
    addon_id uuid NOT NULL REFERENCES local_folders ON DELETE CASCADE,
    path text NOT NULL CHECK (path <> ''),
    size bigint NOT NULL CHECK (size >= 0),
    modified timestamptz NOT NULL,
    text text NOT NULL,
    PRIMARY KEY (addon_id, path)
);
CREATE INDEX local_lyrics_tracks ON local_lyrics (addon_id, lower(regexp_replace(path, '\.[^./]*$', '')));

-- The playlists of a music folder (.m3u and .m3u8 files): title is the
-- name its #PLAYLIST line gives, else its file's; entries are the paths in
-- the folder of the tracks it lists, in order, those leading out of the
-- folder left out. An entry names a track of the folder by its path, in
-- any letter case; those naming none are left out when it is listed.
CREATE TABLE local_playlists (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    addon_id uuid NOT NULL REFERENCES local_folders ON DELETE CASCADE,
    path text NOT NULL CHECK (path <> ''),
    size bigint NOT NULL CHECK (size >= 0),
    modified timestamptz NOT NULL,
    title text NOT NULL,
    entries text[] NOT NULL DEFAULT '{}',
    UNIQUE (addon_id, path)
);
