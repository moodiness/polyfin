-- Whether the titles of a movies or shows folder show the artwork beside
-- their files in place of the metadata addons' (see local_artwork).
ALTER TABLE local_folders
    ADD COLUMN use_artwork boolean NOT NULL DEFAULT true;

-- The artwork a scan found beside the files of a movies or shows folder,
-- each the image of one type of a unit (see local_files): a movie's file
-- or disc, or a show's folder, or with season one of the show's seasons.
-- It is the artwork of the title the unit is matched to. image_type is
-- Jellyfin's; path is the image's, in the folder, with slashes; size and
-- modified tell whether it changed, and give the version apps see of it.
CREATE TABLE local_artwork (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    addon_id uuid NOT NULL REFERENCES local_folders ON DELETE CASCADE,
    unit text NOT NULL CHECK (unit <> ''),
    season integer CHECK (season >= 0),
    image_type text NOT NULL CHECK (image_type IN ('Primary', 'Backdrop', 'Logo', 'Thumb', 'Banner', 'Art', 'Disc')),
    path text NOT NULL CHECK (path <> ''),
    size bigint NOT NULL CHECK (size >= 0),
    modified timestamptz NOT NULL,
    UNIQUE NULLS NOT DISTINCT (addon_id, unit, season, image_type)
);
