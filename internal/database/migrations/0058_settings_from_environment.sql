-- The options POLYFIN_CACHE_SIZE, POLYFIN_VAAPI_DEVICE,
-- POLYFIN_RECORDINGS_DIR, POLYFIN_BACKUP_DIR and POLYFIN_LOG_LEVEL set
-- become settings. cache_size_gb bounds the source cache, in gigabytes;
-- vaapi_device is the render node VAAPI opens, empty for each in turn;
-- recording and backups turn Live TV recording and the daily database
-- backups on, into recordings_folder and backup_folder, absolute paths
-- of at most 512 bytes without control characters, empty for the default
-- folders under POLYFIN_DATA_DIR.
ALTER TABLE settings
    ADD COLUMN cache_size_gb integer NOT NULL DEFAULT 10 CHECK (cache_size_gb BETWEEN 1 AND 2000),
    ADD COLUMN vaapi_device text NOT NULL DEFAULT '' CHECK (vaapi_device = '' OR vaapi_device ~ '^/dev/dri/renderD[0-9]+$'),
    ADD COLUMN recording boolean NOT NULL DEFAULT false,
    ADD COLUMN recordings_folder text NOT NULL DEFAULT '' CHECK (
        recordings_folder = '' OR (recordings_folder LIKE '/%' AND octet_length(recordings_folder) <= 512
            AND recordings_folder !~ '[\x01-\x1f\x7f-\x9f]')
    ),
    ADD COLUMN backups boolean NOT NULL DEFAULT false,
    ADD COLUMN backup_folder text NOT NULL DEFAULT '' CHECK (
        backup_folder = '' OR (backup_folder LIKE '/%' AND octet_length(backup_folder) <= 512
            AND backup_folder !~ '[\x01-\x1f\x7f-\x9f]')
    );

-- Like POLYFIN_HWACCEL and POLYFIN_SEGMENTS before them (see 0049), the
-- variables are copied into the settings at the next start, once, and
-- never read again: environment_pending lists them until then. 'recording'
-- and 'backups' stand for the folder too, which turns them on.
ALTER TABLE settings
    DROP CONSTRAINT settings_environment_pending_check,
    ADD CONSTRAINT settings_environment_pending_check CHECK (
        environment_pending <@ ARRAY['hardware_acceleration', 'segment_order', 'segment_sources_off',
            'cache_size_gb', 'vaapi_device', 'recording', 'backups', 'detailed_log']
    );
UPDATE settings SET environment_pending = environment_pending || ARRAY['cache_size_gb', 'vaapi_device', 'recording', 'backups', 'detailed_log'];
