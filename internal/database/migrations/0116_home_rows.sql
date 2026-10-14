-- The rows the web player adds to its home page (Settings › Web player ›
-- Home rows), in the order of position, at most 20. kind is what a row
-- lists: a catalog of one of the server's addons (addon_id, catalog_type,
-- catalog_id), a collection (collection_id: one made by users, a smart
-- list or an addon's), the titles similar to each of the user's last
-- movies and series (because, one row for each of the last count), or the
-- titles new in the user's libraries (new). place is where the web player
-- shows it: at the top, after Continue Watching, after Next Up or at the
-- bottom. everyone shows it to every user, else to the users of
-- home_row_users. library also makes it a view of its own in /UserViews.
CREATE TABLE home_rows (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    position integer NOT NULL,
    name text NOT NULL CHECK (name <> '' AND char_length(name) <= 100),
    kind text NOT NULL CHECK (kind IN ('catalog', 'collection', 'because', 'new')),
    place text NOT NULL CHECK (place IN ('top', 'resume', 'nextup', 'bottom')),
    addon_id uuid REFERENCES addons (id) ON DELETE CASCADE,
    catalog_type text NOT NULL DEFAULT '',
    catalog_id text NOT NULL DEFAULT '',
    collection_id uuid,
    count integer NOT NULL DEFAULT 3 CHECK (count BETWEEN 1 AND 10),
    everyone boolean NOT NULL DEFAULT true,
    library boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((kind = 'catalog') = (addon_id IS NOT NULL AND catalog_type <> '' AND catalog_id <> '')),
    CHECK ((kind = 'collection') = (collection_id IS NOT NULL))
);

-- The users a row not shown to everyone is shown to.
CREATE TABLE home_row_users (
    row_id uuid NOT NULL REFERENCES home_rows (id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    PRIMARY KEY (row_id, user_id)
);

-- The rows each user turned off (My account › Home rows).
CREATE TABLE home_rows_off (
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    row_id uuid NOT NULL REFERENCES home_rows (id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, row_id)
);

-- How far each catalog was read from its addon, by genre: reach is the
-- position after the last title of the furthest page read, read_at the
-- last read.
CREATE TABLE catalog_reach (
    addon_id uuid NOT NULL REFERENCES addons (id) ON DELETE CASCADE,
    catalog_type text NOT NULL,
    catalog_id text NOT NULL,
    genre text NOT NULL,
    reach integer NOT NULL,
    read_at timestamptz NOT NULL,
    PRIMARY KEY (addon_id, catalog_type, catalog_id, genre)
);

-- When Polyfin first and last saw each title in each catalog it read.
-- arrived is false for the titles that were there before Polyfin read
-- that part of the catalog: only the others are new.
CREATE TABLE catalog_arrivals (
    addon_id uuid NOT NULL,
    catalog_type text NOT NULL,
    catalog_id text NOT NULL,
    genre text NOT NULL,
    item_id uuid NOT NULL,
    first_seen timestamptz NOT NULL,
    last_seen timestamptz NOT NULL,
    arrived boolean NOT NULL,
    PRIMARY KEY (addon_id, catalog_type, catalog_id, genre, item_id),
    FOREIGN KEY (addon_id, catalog_type, catalog_id, genre) REFERENCES catalog_reach ON DELETE CASCADE
);
