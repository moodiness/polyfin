-- The recordings and backup folders are absolute paths of the system
-- Polyfin runs on: /… on Linux and macOS, and on Windows a drive's folder,
-- D:\Recordings or D:/Recordings, or a share's, \\nas\share\Recordings.
-- They were checked to start with / alone, which refused every Windows
-- path the settings take.
ALTER TABLE settings
    DROP CONSTRAINT settings_recordings_folder_check,
    ADD CONSTRAINT settings_recordings_folder_check CHECK (
        recordings_folder = '' OR (recordings_folder ~ '^(/|[A-Za-z]:[\\/]|\\\\)' AND octet_length(recordings_folder) <= 512
            AND recordings_folder !~ '[\x01-\x1f\x7f-\x9f]')
    ),
    DROP CONSTRAINT settings_backup_folder_check,
    ADD CONSTRAINT settings_backup_folder_check CHECK (
        backup_folder = '' OR (backup_folder ~ '^(/|[A-Za-z]:[\\/]|\\\\)' AND octet_length(backup_folder) <= 512
            AND backup_folder !~ '[\x01-\x1f\x7f-\x9f]')
    );
