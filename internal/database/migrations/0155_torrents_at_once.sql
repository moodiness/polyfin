-- How many torrents may run at once, server-wide: 0, the default, for no
-- limit.
ALTER TABLE settings
    ADD COLUMN torrents_at_once integer NOT NULL DEFAULT 0 CHECK (torrents_at_once BETWEEN 0 AND 100);
