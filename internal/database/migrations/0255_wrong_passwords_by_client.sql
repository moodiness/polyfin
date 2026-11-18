-- The wrong passwords in a row each client sent for an account, and until
-- when they block that client's password sign-ins to it: the wrong
-- passwords of one client no longer block an account for every other.
-- client is the client's address, an IPv6 one by its /64; last_at is when
-- its last wrong password came, by which the oldest of an account's
-- clients gives way to a new one past the bound the server keeps.
CREATE TABLE wrong_passwords (
    user_id uuid NOT NULL REFERENCES users ON DELETE CASCADE,
    client text NOT NULL,
    attempts integer NOT NULL CHECK (attempts > 0),
    blocked_until timestamptz,
    last_at timestamptz NOT NULL,
    PRIMARY KEY (user_id, client)
);

-- users.invalid_login_attempts and users.blocked_until now hold the
-- highest count and the latest block among an account's clients, for what
-- shows them. The counts and blocks in force were of whole accounts: they
-- end, since no client is known for them.
UPDATE users SET invalid_login_attempts = 0, blocked_until = NULL
    WHERE invalid_login_attempts <> 0 OR blocked_until IS NOT NULL;
