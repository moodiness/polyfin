-- How often, in minutes, connected accounts are imported again: the app
-- connections and the watch histories of tracking services. 6 hours by
-- default.
ALTER TABLE settings
    ADD COLUMN import_every_minutes integer NOT NULL DEFAULT 360
        CHECK (import_every_minutes IN (15, 30, 60, 120, 180, 360, 720, 1440));
