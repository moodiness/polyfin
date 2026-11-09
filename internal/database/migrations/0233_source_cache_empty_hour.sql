-- The hour of the server's time zone at which the source cache is emptied
-- each day, but for the files being read, -1 for never.
ALTER TABLE settings
    ADD COLUMN source_cache_empty_hour integer NOT NULL DEFAULT -1 CHECK (source_cache_empty_hour BETWEEN -1 AND 23);
