-- Scrubbing thumbnails (Jellyfin's trickplay) and chapter images, made
-- from the keyframes of the versions played: whether each is made, one
-- thumbnail every trickplay_interval seconds, trickplay_width pixels wide,
-- and the space, in GB, the images of every version take at most. Both
-- read parts of the sources, so both start turned off.
ALTER TABLE settings
    ADD COLUMN trickplay boolean NOT NULL DEFAULT false,
    ADD COLUMN trickplay_interval integer NOT NULL DEFAULT 10 CHECK (trickplay_interval BETWEEN 5 AND 60),
    ADD COLUMN trickplay_width integer NOT NULL DEFAULT 320 CHECK (trickplay_width IN (240, 320, 480)),
    ADD COLUMN chapter_images boolean NOT NULL DEFAULT false,
    ADD COLUMN thumbnail_storage_gb integer NOT NULL DEFAULT 2 CHECK (thumbnail_storage_gb BETWEEN 1 AND 50);

-- The versions images were made of: the title each belongs to, when its
-- images were last used, and the bytes they take. Past the storage cap,
-- the versions used longest ago lose theirs.
CREATE TABLE thumbnail_versions (
    version_id uuid PRIMARY KEY,
    item_id uuid NOT NULL,
    used_at timestamptz NOT NULL DEFAULT now(),
    bytes bigint NOT NULL DEFAULT 0 CHECK (bytes >= 0)
);
CREATE INDEX thumbnail_versions_item ON thumbnail_versions (item_id);
CREATE INDEX thumbnail_versions_used ON thumbnail_versions (used_at);

-- A version's scrubbing thumbnails at one width, as Jellyfin describes
-- them (TrickplayInfo), and the tiles packing them.
CREATE TABLE trickplay_sets (
    version_id uuid NOT NULL REFERENCES thumbnail_versions ON DELETE CASCADE,
    width integer NOT NULL CHECK (width > 0),
    height integer NOT NULL CHECK (height > 0),
    tile_width integer NOT NULL CHECK (tile_width > 0),
    tile_height integer NOT NULL CHECK (tile_height > 0),
    thumbnail_count integer NOT NULL CHECK (thumbnail_count > 0),
    interval_ms integer NOT NULL CHECK (interval_ms > 0),
    bandwidth integer NOT NULL CHECK (bandwidth >= 0),
    PRIMARY KEY (version_id, width)
);
CREATE TABLE trickplay_tiles (
    version_id uuid NOT NULL,
    width integer NOT NULL,
    tile integer NOT NULL CHECK (tile >= 0),
    data bytea NOT NULL,
    PRIMARY KEY (version_id, width, tile),
    FOREIGN KEY (version_id, width) REFERENCES trickplay_sets ON DELETE CASCADE
);

-- An image of each chapter of a version, at the keyframe nearest its
-- start; tag changes when the image does.
CREATE TABLE chapter_images (
    version_id uuid NOT NULL REFERENCES thumbnail_versions ON DELETE CASCADE,
    chapter integer NOT NULL CHECK (chapter >= 0),
    tag text NOT NULL,
    data bytea NOT NULL,
    made_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (version_id, chapter)
);
