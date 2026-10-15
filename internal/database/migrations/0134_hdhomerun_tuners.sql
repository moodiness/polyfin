-- An IPTV source may be an HDHomeRun tuner on the local network: its address
-- is the tuner's, its channels come from its line-up, and its tuner count is
-- its connection limit (max_connections).
ALTER TABLE addons
    DROP CONSTRAINT addons_kind_check,
    ADD CONSTRAINT addons_kind_check CHECK (kind IN ('stremio', 'm3u', 'xtream', 'eclipse', 'local', 'hdhomerun'));

-- What a tuner says of itself (its device identifier and model), whether
-- only the channels marked favorite on it are kept, off by default, and the
-- last failure of its streams other than its tuners being in use, which
-- System › Health shows until a stream plays again.
ALTER TABLE iptv_sources
    ADD COLUMN tuner_device text NOT NULL DEFAULT '',
    ADD COLUMN tuner_model text NOT NULL DEFAULT '',
    ADD COLUMN favorites_only boolean NOT NULL DEFAULT false,
    ADD COLUMN tuner_failure text NOT NULL DEFAULT '',
    ADD COLUMN tuner_failed_at timestamptz;

-- A tuner's channels are numbered as it numbers them ("5.1"), and may be
-- marked favorite on it.
ALTER TABLE iptv_entries
    ADD COLUMN guide_number text NOT NULL DEFAULT '',
    ADD COLUMN favorite boolean NOT NULL DEFAULT false;

-- The tuner's number of a line-up channel, shown while the channel has no
-- fixed number and the source keeps the provider's numbers.
ALTER TABLE iptv_lineup
    ADD COLUMN provider_guide_number text NOT NULL DEFAULT '';
