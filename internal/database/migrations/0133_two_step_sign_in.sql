-- Two-step sign-in with an authenticator app. two_step is on once a code
-- from the app confirmed two_step_secret, the TOTP secret, sealed with
-- POLYFIN_SECRET_KEY as the other stored secrets; while off, the secret is
-- the one being set up, if any. two_step_step is the last time step a code
-- was accepted for, so that a code is accepted once; two_step_recovery
-- holds the SHA-256 hashes of the recovery codes left.
ALTER TABLE users
    ADD COLUMN two_step boolean NOT NULL DEFAULT false,
    ADD COLUMN two_step_secret text NOT NULL DEFAULT '',
    ADD COLUMN two_step_step bigint NOT NULL DEFAULT 0,
    ADD COLUMN two_step_recovery bytea[] NOT NULL DEFAULT '{}';

-- Administrators must use two-step sign-in. Off by default.
ALTER TABLE settings
    ADD COLUMN two_step_administrators boolean NOT NULL DEFAULT false;
