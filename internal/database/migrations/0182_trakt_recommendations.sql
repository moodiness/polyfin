-- The movies and series Trakt recommends to each user who connected it,
-- for the Recommended on Trakt home row: the items found for them, best
-- first; when they are read again (due_at, null until the first read
-- ends), and when a row last asked for them (asked_at), as only those
-- asked for lately are read again on schedule. Disconnecting, or
-- connecting again, forgets them.
CREATE TABLE tracking_recommendations (
    user_id uuid NOT NULL,
    service text NOT NULL,
    items uuid[] NOT NULL DEFAULT '{}' CHECK (cardinality(items) <= 500),
    due_at timestamptz,
    asked_at timestamptz NOT NULL,
    PRIMARY KEY (user_id, service),
    FOREIGN KEY (user_id, service) REFERENCES tracking_connections ON DELETE CASCADE
);

-- A home row may list the movies and series Trakt recommends to the user
-- (trakt).
ALTER TABLE home_rows
    DROP CONSTRAINT home_rows_kind_check,
    ADD CONSTRAINT home_rows_kind_check CHECK (kind IN ('catalog', 'collection', 'because', 'new', 'available', 'waiting', 'trakt'));
