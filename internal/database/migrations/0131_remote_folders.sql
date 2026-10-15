-- A local folder may also be a folder of an S3 bucket, s3://bucket/prefix,
-- reached at share_endpoint and signed for share_region with share_user and
-- share_password as access key and secret key; a folder of an SFTP server,
-- sftp://host[:port]/path, signed in as share_user with share_password or
-- share_private_key, opened with share_passphrase, the server's host key
-- saved in share_host_key at the first connection and another one it
-- offers later kept in share_offered_key until an administrator accepts
-- it; or a folder of share_remote, an rclone remote, through the rclone
-- instance rclone://host[:port]/path names. The keys and the passphrase
-- are sealed with POLYFIN_SECRET_KEY as the passwords are. poll_minutes is
-- how often such a folder is listed again to find its changes, 0 only at
-- its scans, NULL for the other folders. A server offering another host
-- key fails the scans with host_key_changed.
ALTER TABLE local_folders
    ADD COLUMN share_endpoint text NOT NULL DEFAULT '',
    ADD COLUMN share_region text NOT NULL DEFAULT '',
    ADD COLUMN share_remote text NOT NULL DEFAULT '',
    ADD COLUMN share_private_key text NOT NULL DEFAULT '',
    ADD COLUMN share_passphrase text NOT NULL DEFAULT '',
    ADD COLUMN share_host_key text NOT NULL DEFAULT '',
    ADD COLUMN share_offered_key text NOT NULL DEFAULT '',
    ADD COLUMN poll_minutes integer CHECK (poll_minutes BETWEEN 0 AND 1440),
    DROP CONSTRAINT local_folders_error_check,
    ADD CONSTRAINT local_folders_error_check
        CHECK (error IN ('', 'missing', 'unreadable', 'not_folder', 'unreachable', 'refused', 'host_key_changed'));
