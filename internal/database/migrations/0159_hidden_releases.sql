-- The kinds of low-quality releases the version rules hide, by the tags
-- their labels hold: camera copies, telesyncs, telecines, screeners and
-- early copies; none by default.
ALTER TABLE settings
    ADD COLUMN hidden_releases text[] NOT NULL DEFAULT '{}'
        CHECK (hidden_releases <@ '{cam,telesync,telecine,screener,early}');
