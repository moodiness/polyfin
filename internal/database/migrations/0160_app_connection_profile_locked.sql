-- A connection's import may find that the profile chosen now asks for a PIN
-- Polyfin does not have: one set since it was chosen, or another, as the
-- app refuses the profile's session Polyfin kept ('profile_locked'). The
-- user signs in again to choose it.
ALTER TABLE app_connections
    DROP CONSTRAINT app_connections_last_problem_check,
    ADD CONSTRAINT app_connections_last_problem_check CHECK (last_problem IN ('unreachable', 'rate_limited', 'unexpected',
        'profile_missing', 'profile_locked', 'sign_in_refused', 'internal'));
