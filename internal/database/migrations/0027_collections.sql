-- Collections users make of titles, as Jellyfin's BoxSets: every user sees
-- them; those allowed to manage collections create, change and delete them.
CREATE TABLE collections (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (length(name) <= 500),
    -- Jellyfin's IsLocked: the collection's metadata is not looked up.
    is_locked boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    -- When titles were last added, NULL before any.
    last_added_at timestamptz
);

-- The titles of a collection, in the order they were added; a title is in
-- a collection once.
CREATE TABLE collection_items (
    collection_id uuid NOT NULL REFERENCES collections (id) ON DELETE CASCADE,
    item_id uuid NOT NULL,
    ordinal bigint GENERATED ALWAYS AS IDENTITY,
    PRIMARY KEY (collection_id, item_id)
);
CREATE INDEX collection_items_item ON collection_items (item_id);

-- Whether each user may manage collections, as Jellyfin's
-- EnableCollectionManagement. Administrators may until it is taken away.
ALTER TABLE users ADD COLUMN collection_management boolean NOT NULL DEFAULT false;
UPDATE users SET collection_management = is_administrator;
