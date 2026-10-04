-- What administrators change of an item in the metadata editor of Jellyfin
-- apps: the fields they set, which win over the addons' values for every
-- user, and the artwork they upload. Items are named by the identifier
-- Jellyfin apps know them by; libraries and collections are not all in
-- items, so there is no foreign key.
CREATE TABLE item_overrides (
    item_id uuid PRIMARY KEY,
    fields jsonb NOT NULL CHECK (jsonb_typeof(fields) = 'object'),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE item_images (
    item_id uuid NOT NULL,
    image_type text NOT NULL CHECK (image_type IN ('Primary', 'Backdrop', 'Logo', 'Thumb', 'Banner')),
    image bytea NOT NULL CHECK (octet_length(image) BETWEEN 1 AND 10485760),
    content_type text NOT NULL,
    tag text NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (item_id, image_type)
);
