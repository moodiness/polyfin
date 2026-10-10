-- Whether Polyfin asks GitHub once a day for its latest release, to tell
-- administrators about a new version.
ALTER TABLE settings
    ADD COLUMN update_check boolean NOT NULL DEFAULT true;

-- What the daily check last found, one row once it found a release: the
-- latest version and the address of its release notes, and the version
-- administrators were last told about, so that a restart tells none twice.
CREATE TABLE update_check (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    latest_version text NOT NULL DEFAULT '',
    release_url text NOT NULL DEFAULT '',
    notified_version text NOT NULL DEFAULT '',
    checked_at timestamptz
);
