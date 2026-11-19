-- A share's scan that lasts longer than one scan may is stopped, and the
-- folder fails with too_slow.
ALTER TABLE local_folders
    DROP CONSTRAINT local_folders_error_check,
    ADD CONSTRAINT local_folders_error_check
        CHECK (error IN ('', 'missing', 'unreadable', 'not_folder', 'unreachable', 'refused', 'host_key_changed', 'too_deep',
            'too_many_folders', 'too_many_files', 'listing_too_large', 'too_slow'));
