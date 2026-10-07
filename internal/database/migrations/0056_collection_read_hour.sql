-- The hour of the server's time zone at which every collection of the
-- server's collection libraries is read each day, -1 for never.
ALTER TABLE settings
    ADD COLUMN collection_read_hour integer NOT NULL DEFAULT -1 CHECK (collection_read_hour BETWEEN -1 AND 23);
