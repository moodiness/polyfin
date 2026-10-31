-- The units of a local folder (see local_files) that Identify relinked
-- from the title their files were matched to, former, a Stremio
-- identifier: the item of that title, last listed by the folder, answers
-- the title its files are matched to now while none of the folder's files
-- is matched to it, as Jellyfin keeps an item's identifier when it is
-- identified again (see localfiles.Service.Relinked).
CREATE TABLE local_relinked_units (
    addon_id uuid NOT NULL REFERENCES local_folders ON DELETE CASCADE,
    unit text NOT NULL CHECK (unit <> ''),
    former text NOT NULL CHECK (former <> ''),
    PRIMARY KEY (addon_id, unit)
);
