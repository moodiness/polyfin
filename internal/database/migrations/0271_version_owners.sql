-- The item each version listed belongs to, so that an app opening a
-- version as an item by its identifier finds it after a restart, before
-- the item is listed again.
CREATE TABLE version_owners (
    version_id uuid PRIMARY KEY,
    item_id uuid NOT NULL,
    seen_at timestamptz NOT NULL
);
CREATE INDEX version_owners_seen ON version_owners (seen_at);
