-- Whether the check for new versions, and Polyfin Desktop's updates,
-- consider pre-releases too.
ALTER TABLE settings
    ADD COLUMN include_prereleases boolean NOT NULL DEFAULT false;

-- The latest pre-release the check found newer than the latest release,
-- while pre-releases were included: its version and the address of its
-- release notes, empty otherwise.
ALTER TABLE update_check
    ADD COLUMN prerelease_version text NOT NULL DEFAULT '',
    ADD COLUMN prerelease_url text NOT NULL DEFAULT '';
