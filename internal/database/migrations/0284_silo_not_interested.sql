-- The movies and series each user marked Not interested in Silo's apps,
-- with when (see package silo). A marked title leaves that user's For You
-- and similar titles in Silo's apps only; it stays in their listings,
-- searches and Jellyfin apps.
CREATE TABLE silo_not_interested (
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    item_id uuid NOT NULL,
    marked_at timestamptz NOT NULL,
    PRIMARY KEY (user_id, item_id)
);
