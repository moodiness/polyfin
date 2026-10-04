-- An addon may also be an Eclipse music addon, whose manifest is kept as
-- Eclipse writes it.
ALTER TABLE addons DROP CONSTRAINT addons_kind_check;
ALTER TABLE addons
    ADD CONSTRAINT addons_kind_check CHECK (kind IN ('stremio', 'm3u', 'xtream', 'eclipse'));

-- The values chosen for an Eclipse addon's settings, by key, sent with
-- every request to it; a setting missing here is sent with its default.
ALTER TABLE addons
    ADD COLUMN settings jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(settings) = 'object');
