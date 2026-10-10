-- A local folder may be a network share, read by Polyfin itself: its
-- address, the addon's manifest URL, is smb://host[:port]/share[/path] or
-- http(s)://host/path for WebDAV, read as share_user with share_password,
-- sealed with POLYFIN_SECRET_KEY like the other stored secrets. A share
-- that cannot be reached, or that refuses the user or password, fails its
-- scans with errors of its own.
ALTER TABLE local_folders
    ADD COLUMN share_user text NOT NULL DEFAULT '',
    ADD COLUMN share_password text NOT NULL DEFAULT '',
    DROP CONSTRAINT local_folders_error_check,
    ADD CONSTRAINT local_folders_error_check
        CHECK (error IN ('', 'missing', 'unreadable', 'not_folder', 'unreachable', 'refused'));
