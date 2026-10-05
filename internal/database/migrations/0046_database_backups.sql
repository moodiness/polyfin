-- The hour of the server's time zone the database is backed up at every
-- day, when POLYFIN_BACKUP_DIR names a folder, and how many backups are
-- kept there.
ALTER TABLE settings
    ADD COLUMN backup_hour integer NOT NULL DEFAULT 4 CHECK (backup_hour BETWEEN 0 AND 23),
    ADD COLUMN backups_kept integer NOT NULL DEFAULT 7 CHECK (backups_kept BETWEEN 1 AND 90);

-- How the last backup went, one row once one has run: when the last run
-- started and its error, empty when it succeeded; and the last backup made,
-- when it started, its file name and its size in bytes, which a failure
-- after it keeps.
CREATE TABLE backup_status (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    ran_at timestamptz NOT NULL,
    error text NOT NULL DEFAULT '',
    made_at timestamptz,
    file text NOT NULL DEFAULT '',
    size bigint NOT NULL DEFAULT 0 CHECK (size >= 0)
);
