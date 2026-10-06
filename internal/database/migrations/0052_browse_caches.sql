-- What addons answered for browsing, kept across restarts so that a home
-- screen, a library or a title page answers at once after one: each page
-- of a catalog and each title's description, with when the addon gave it.
-- Polyfin shows them while it asks the addon again in the background, and
-- forgets those not read for a day past their refresh age (pages) or a
-- week (descriptions). config tells the addon's address they were asked
-- at, hashed: an addon given another address (another configuration)
-- answers anew.
CREATE TABLE catalog_pages (
    addon_id uuid NOT NULL REFERENCES addons (id) ON DELETE CASCADE,
    catalog_type text NOT NULL,
    catalog_id text NOT NULL,
    genre text NOT NULL,
    skip integer NOT NULL CHECK (skip >= 0),
    config text NOT NULL,
    metas jsonb NOT NULL CHECK (jsonb_typeof(metas) = 'array'),
    fetched_at timestamptz NOT NULL,
    PRIMARY KEY (addon_id, catalog_type, catalog_id, genre, skip)
);

CREATE INDEX catalog_pages_fetched_at ON catalog_pages (fetched_at);

CREATE TABLE metas (
    addon_id uuid NOT NULL REFERENCES addons (id) ON DELETE CASCADE,
    meta_type text NOT NULL,
    meta_id text NOT NULL,
    config text NOT NULL,
    meta jsonb NOT NULL CHECK (jsonb_typeof(meta) = 'object'),
    fetched_at timestamptz NOT NULL,
    PRIMARY KEY (addon_id, meta_type, meta_id)
);

CREATE INDEX metas_fetched_at ON metas (fetched_at);

-- The number of titles a page of each catalog holds, the longest page seen,
-- so that a catalog not read for long is asked for all the pages a listing
-- needs at once.
CREATE TABLE catalog_sizes (
    addon_id uuid NOT NULL REFERENCES addons (id) ON DELETE CASCADE,
    catalog_type text NOT NULL,
    catalog_id text NOT NULL,
    page_size integer NOT NULL CHECK (page_size > 0),
    PRIMARY KEY (addon_id, catalog_type, catalog_id)
);
