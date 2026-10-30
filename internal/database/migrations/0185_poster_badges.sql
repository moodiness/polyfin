-- Badges over movies' and series' posters, for every app: their ratings,
-- and a series' new episode or season, at a size preset; and the address
-- template of posters in place of the addons' ones. Both off by default.
ALTER TABLE settings
    ADD COLUMN poster_badges boolean NOT NULL DEFAULT false,
    ADD COLUMN poster_badge_size text NOT NULL DEFAULT 'medium',
    ADD COLUMN poster_template text NOT NULL DEFAULT '';
