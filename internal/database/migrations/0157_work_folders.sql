-- The folders the segments of conversions and repackagings, the files
-- being read, and the cache (relayed images, guides and picture subtitles
-- being read) are kept in: absolute paths of the system Polyfin runs on,
-- as the recordings and backup folders are, or '', the default, for the
-- cache in the data folder.
ALTER TABLE settings
    ADD COLUMN conversion_folder text NOT NULL DEFAULT '' CHECK (
        conversion_folder = '' OR (conversion_folder ~ '^(/|[A-Za-z]:[\\/]|\\\\)' AND octet_length(conversion_folder) <= 512
            AND conversion_folder !~ '[\x01-\x1f\x7f-\x9f]')
    ),
    ADD COLUMN reading_folder text NOT NULL DEFAULT '' CHECK (
        reading_folder = '' OR (reading_folder ~ '^(/|[A-Za-z]:[\\/]|\\\\)' AND octet_length(reading_folder) <= 512
            AND reading_folder !~ '[\x01-\x1f\x7f-\x9f]')
    ),
    ADD COLUMN cache_folder text NOT NULL DEFAULT '' CHECK (
        cache_folder = '' OR (cache_folder ~ '^(/|[A-Za-z]:[\\/]|\\\\)' AND octet_length(cache_folder) <= 512
            AND cache_folder !~ '[\x01-\x1f\x7f-\x9f]')
    );
