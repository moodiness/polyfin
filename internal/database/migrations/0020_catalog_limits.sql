-- How many items one read of a catalog fetches at most: some catalogs are
-- nearly endless. Live TV catalogs, which list channels and, for the guide,
-- each day's programmes, have their own limit.
ALTER TABLE settings
    ADD COLUMN catalog_limit integer NOT NULL DEFAULT 2000 CHECK (catalog_limit BETWEEN 100 AND 20000),
    ADD COLUMN channel_limit integer NOT NULL DEFAULT 10000 CHECK (channel_limit BETWEEN 100 AND 50000);
