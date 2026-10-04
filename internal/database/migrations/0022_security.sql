-- Security settings: whether users' own addons are used at all; how many
-- wrong passwords in a row block an account for a while (0: never); after
-- how many days unused a Jellyfin app is signed out (0: never); and whether
-- Polyfin logs in detail, at the debug level.
ALTER TABLE settings
    ADD COLUMN personal_addons boolean NOT NULL DEFAULT true,
    ADD COLUMN login_attempts integer NOT NULL DEFAULT 0
        CHECK (login_attempts = 0 OR login_attempts BETWEEN 3 AND 20),
    ADD COLUMN inactive_device_days integer NOT NULL DEFAULT 0
        CHECK (inactive_device_days BETWEEN 0 AND 365),
    ADD COLUMN detailed_log boolean NOT NULL DEFAULT false;

-- Whether a user may add and use their own addons, granted unless taken
-- away; their wrong passwords in a row, and until when they block the
-- account.
ALTER TABLE users
    ADD COLUMN personal_addons boolean NOT NULL DEFAULT true,
    ADD COLUMN invalid_login_attempts integer NOT NULL DEFAULT 0 CHECK (invalid_login_attempts >= 0),
    ADD COLUMN blocked_until timestamptz;
