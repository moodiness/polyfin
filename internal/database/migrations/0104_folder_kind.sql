-- The catalog types the libraries of an addon's catalogs take their
-- identifiers from, by catalog identifier, for the catalogs whose type
-- changed since their library was made: a local folder holding something
-- else keeps its library, which apps know by its identifier (see
-- addons.Library.KeyType). A catalog not listed takes them from its own.
ALTER TABLE addons ADD COLUMN library_key_types jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(library_key_types) = 'object');

-- The items each file of a local folder was a title of before what the
-- folder holds changed between movies, shows and home videos: what users
-- did with them follows the file to the title a scan matches it to as
-- the new kind (see localfiles.follow). A row goes once followed, or with
-- its file.
CREATE TABLE local_former_titles (
    addon_id uuid NOT NULL REFERENCES local_folders ON DELETE CASCADE,
    path text NOT NULL,
    item_id uuid NOT NULL,
    PRIMARY KEY (addon_id, path, item_id)
);
