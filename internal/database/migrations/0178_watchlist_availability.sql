-- What the checks of each user's watchlist found (see package watchlist):
-- for each title checked, when it was last checked, whether versions the
-- user may play were found then, since when they have been found without
-- a break, and when they were first found, kept once the title loses them
-- so that it is told of once only. The Now available and Still waiting
-- rows list the titles of the watchlist the user has not started by it.
CREATE TABLE watchlist_availability (
    user_id uuid NOT NULL REFERENCES users ON DELETE CASCADE,
    item_id uuid NOT NULL,
    checked_at timestamptz NOT NULL,
    available boolean NOT NULL,
    available_since timestamptz,
    first_available_at timestamptz,
    PRIMARY KEY (user_id, item_id),
    CHECK (available = (available_since IS NOT NULL)),
    CHECK (first_available_at IS NOT NULL OR NOT available)
);

-- How many hours after its last check a title of a watchlist is checked
-- again for versions, 0 for never: by default every 6 hours.
ALTER TABLE settings
    ADD COLUMN watchlist_check_hours integer NOT NULL DEFAULT 6 CHECK (watchlist_check_hours BETWEEN 0 AND 168);

-- A home row may list the titles of the user's watchlist that versions
-- were found for (available), or those still without (waiting).
ALTER TABLE home_rows
    DROP CONSTRAINT home_rows_kind_check,
    ADD CONSTRAINT home_rows_kind_check CHECK (kind IN ('catalog', 'collection', 'because', 'new', 'available', 'waiting'));
