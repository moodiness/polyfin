-- Polyfin keeps the addons, libraries and guides of every scope in memory
-- (see addons.Store). Any change to them, whoever makes it, tells the
-- server to read them again once its transaction commits; PostgreSQL sends
-- one notification per transaction.
CREATE FUNCTION notify_addons_changed() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_notify('polyfin_addons', '');
    RETURN NULL;
END
$$;
CREATE TRIGGER addons_changed AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON addons
    FOR EACH STATEMENT EXECUTE FUNCTION notify_addons_changed();
CREATE TRIGGER libraries_changed AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON libraries
    FOR EACH STATEMENT EXECUTE FUNCTION notify_addons_changed();
CREATE TRIGGER live_guides_changed AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON live_guides
    FOR EACH STATEMENT EXECUTE FUNCTION notify_addons_changed();

-- New defaults: playback is prepared in advance, an analysis is given up
-- after 20 seconds, and catalogs are read again every hour. Servers still
-- on the old defaults move to the new ones; another value was chosen, and
-- is kept.
ALTER TABLE settings
    ALTER COLUMN prepare_ahead SET DEFAULT true,
    ALTER COLUMN analysis_timeout SET DEFAULT 20,
    ALTER COLUMN catalog_refresh_minutes SET DEFAULT 60;
UPDATE settings SET prepare_ahead = true WHERE NOT prepare_ahead;
UPDATE settings SET analysis_timeout = 20 WHERE analysis_timeout = 45;
UPDATE settings SET catalog_refresh_minutes = 60 WHERE catalog_refresh_minutes = 10;

-- Chapters are always sent, and downloads follow each user's own
-- permission alone. Where the server turned downloads off, every user
-- loses the permission, so that nobody can download who could not.
UPDATE users SET content_downloading = false
    WHERE content_downloading AND NOT (SELECT downloads FROM settings);
ALTER TABLE settings
    DROP COLUMN chapters,
    DROP COLUMN downloads;

-- POLYFIN_HWACCEL and POLYFIN_SEGMENTS become the defaults of a new
-- server: Polyfin copies them into the settings at its next start, once,
-- and never reads them again for these settings. environment_pending
-- lists the settings still to be copied: the GPU where it followed
-- POLYFIN_HWACCEL (''), the order of the segment databases where it
-- followed POLYFIN_SEGMENTS ('{}'), and always the databases turned off,
-- which POLYFIN_SEGMENTS alone chose. Until then, they hold the defaults.
-- segment_sources_off lists the segment databases never asked, each once,
-- wherever they are in segment_order, which holds every database once.
ALTER TABLE settings
    ADD COLUMN segment_sources_off text[] NOT NULL DEFAULT '{}' CHECK (
        segment_sources_off <@ ARRAY['theintrodb', 'introdb', 'publicmetadb']
        AND cardinality(array_positions(segment_sources_off, 'theintrodb')) <= 1
        AND cardinality(array_positions(segment_sources_off, 'introdb')) <= 1
        AND cardinality(array_positions(segment_sources_off, 'publicmetadb')) <= 1
    ),
    ADD COLUMN environment_pending text[] NOT NULL DEFAULT '{}' CHECK (
        environment_pending <@ ARRAY['hardware_acceleration', 'segment_order', 'segment_sources_off']
    );
UPDATE settings SET
    environment_pending = array_remove(ARRAY[
        CASE WHEN hardware_acceleration = '' THEN 'hardware_acceleration' END,
        CASE WHEN segment_order = '{}' THEN 'segment_order' END,
        'segment_sources_off'
    ], NULL),
    hardware_acceleration = CASE WHEN hardware_acceleration = '' THEN 'auto' ELSE hardware_acceleration END,
    segment_order = CASE WHEN segment_order = '{}' THEN ARRAY['theintrodb', 'introdb', 'publicmetadb'] ELSE segment_order END;
ALTER TABLE settings
    DROP CONSTRAINT settings_hardware_acceleration_check,
    ADD CONSTRAINT settings_hardware_acceleration_check CHECK (hardware_acceleration IN ('auto', 'nvenc', 'vaapi', 'none')),
    ALTER COLUMN hardware_acceleration SET DEFAULT 'auto',
    DROP CONSTRAINT settings_segment_order_check,
    ADD CONSTRAINT settings_segment_order_check CHECK (
        cardinality(segment_order) = 3 AND segment_order @> ARRAY['theintrodb', 'introdb', 'publicmetadb']
    ),
    ALTER COLUMN segment_order SET DEFAULT ARRAY['theintrodb', 'introdb', 'publicmetadb'];
