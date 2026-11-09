-- The exact time an account ends, NULL for none: an invite link with an
-- account end gives the accounts it creates one, that many days of 24
-- hours after their creation, while an administrator sets a date
-- (ends_on). An account has one or the other, and is refused from it as a
-- disabled one is. An administrator's account never ends.
ALTER TABLE users
    ADD COLUMN ends_at timestamptz,
    ADD CONSTRAINT users_one_end CHECK (ends_on IS NULL OR ends_at IS NULL),
    DROP CONSTRAINT users_administrators_never_end,
    ADD CONSTRAINT users_administrators_never_end CHECK (NOT is_administrator OR (ends_on IS NULL AND ends_at IS NULL));
