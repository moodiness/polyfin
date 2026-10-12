-- The date an account ends, in the server's time zone, NULL for never:
-- from that date it is refused as a disabled one is. An administrator's
-- account never ends. end_recorded is set once the activity log recorded
-- that the account ended, and cleared by a new end date.
ALTER TABLE users
    ADD COLUMN ends_on date,
    ADD COLUMN end_recorded boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT users_administrators_never_end CHECK (NOT is_administrator OR ends_on IS NULL);

-- How many days after its creation each account an invite creates ends,
-- NULL for never.
ALTER TABLE invites ADD COLUMN account_days integer CHECK (account_days > 0);
