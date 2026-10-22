-- Pre-release rips (CAM, telesync, telecine, screener, workprint) are
-- excluded from every title's versions, each kind on its own; all of them
-- by default, existing servers included.
ALTER TABLE settings
    ADD COLUMN hidden_rip_types text[] NOT NULL DEFAULT '{cam,telesync,telecine,screener,workprint}'
        CHECK (hidden_rip_types <@ '{cam,telesync,telecine,screener,workprint}');
