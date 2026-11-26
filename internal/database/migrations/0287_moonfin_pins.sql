-- The PIN that locks a user's profile on the user picker of Moonfin's apps
-- (see package moonfin), kept on the server so that a PIN set on one
-- device holds on all of them. hash is a salted Argon2id hash of the PIN,
-- as a password's, never the PIN itself. wrong_guesses counts the wrong
-- PINs given since the last right one, and wait_until, past the free
-- ones, is when the next guess may be checked. A user without a PIN has
-- no row.
CREATE TABLE moonfin_pins (
    user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    hash text NOT NULL CHECK (hash <> ''),
    wrong_guesses integer NOT NULL DEFAULT 0 CHECK (wrong_guesses >= 0),
    wait_until timestamptz,
    updated_at timestamptz NOT NULL
);
