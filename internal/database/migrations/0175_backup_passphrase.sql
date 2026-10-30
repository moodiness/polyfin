-- The passphrase database backups are encrypted with, empty to write them
-- unencrypted. It is sealed with POLYFIN_SECRET_KEY as the other stored
-- secrets, so its length is not bounded here.
ALTER TABLE settings
    ADD COLUMN backup_passphrase text NOT NULL DEFAULT '';
