-- A library's image may also be one Polyfin draws from its first titles
-- ('generated'). Live TV catalogs still make no library: they have none.
-- The check added with the column has a generated name, shared by the
-- table's other checks on several columns: it is found by what it allows.
DO $$
DECLARE
    found text;
BEGIN
    SELECT conname INTO STRICT found FROM pg_constraint
        WHERE conrelid = 'libraries'::regclass AND contype = 'c' AND pg_get_constraintdef(oid) LIKE '%''automatic''%';
    EXECUTE format('ALTER TABLE libraries DROP CONSTRAINT %I', found);
END
$$;
ALTER TABLE libraries
    ADD CONSTRAINT libraries_image_check
        CHECK (image IN ('none', 'automatic', 'generated') AND (image = 'none' OR catalog_type <> 'tv'));

-- generate_images gives the addons' collections without a poster, and the
-- smart lists shown as collections, an image Polyfin draws.
ALTER TABLE settings
    ADD COLUMN generate_images boolean NOT NULL DEFAULT true;

-- What the image Polyfin draws for an item shows: its name, and its first
-- titles with a poster, in order, with their posters, so that it is drawn
-- again only when they change (see library.generatedChoice). confined
-- downloads the posters from public addresses only.
CREATE TABLE generated_images (
    item_id uuid PRIMARY KEY,
    name text NOT NULL,
    titles uuid[] NOT NULL,
    posters text[] NOT NULL CHECK (cardinality(posters) = cardinality(titles)),
    confined boolean NOT NULL DEFAULT false,
    chosen_at timestamptz NOT NULL DEFAULT now()
);
