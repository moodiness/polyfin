-- A share that holds more than one scan reads fails its scans with the
-- bound it met: too_deep when its folders go deeper than a scan follows
-- them, too_many_folders and too_many_files when it holds more of them
-- than a scan reads, listing_too_large when one of its folders lists
-- more than is read of one folder.
ALTER TABLE local_folders
    DROP CONSTRAINT local_folders_error_check,
    ADD CONSTRAINT local_folders_error_check
        CHECK (error IN ('', 'missing', 'unreadable', 'not_folder', 'unreachable', 'refused', 'host_key_changed', 'too_deep',
            'too_many_folders', 'too_many_files', 'listing_too_large'));
