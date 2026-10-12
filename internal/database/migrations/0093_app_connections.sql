-- The app accounts users stay connected to, one per app, which Polyfin
-- imports again every 6 hours (see appimport). sign_in is what reads the
-- account again, sealed with POLYFIN_SECRET_KEY when it is set: Stremio's
-- auth key, or Nuvio's backend address, profile and refresh token. owned
-- tells that Polyfin opened the session, which disconnecting signs out; a
-- key the user pasted is only forgotten. account is the email or name the
-- app gave, empty when none; connected_by is who connected it: the user,
-- or an administrator. problem is 'sign_in_refused' once the app refused
-- the sign-in kept: nothing is imported until the user signs in again.
-- The last import: when it ended, the entries it read, the titles it
-- marked played, the resume points it set, the favorites it added, the
-- titles it did not find, and what kept it from reading the account.
CREATE TABLE app_connections (
    user_id uuid NOT NULL REFERENCES users ON DELETE CASCADE,
    app text NOT NULL CHECK (app IN ('nuvio', 'stremio')),
    sign_in text NOT NULL CHECK (sign_in <> ''),
    owned boolean NOT NULL,
    account text NOT NULL DEFAULT '',
    connected_at timestamptz NOT NULL,
    connected_by uuid REFERENCES users ON DELETE SET NULL,
    problem text CHECK (problem IN ('sign_in_refused')),
    last_import_at timestamptz,
    read integer NOT NULL DEFAULT 0 CHECK (read >= 0),
    played integer NOT NULL DEFAULT 0 CHECK (played >= 0),
    resumed integer NOT NULL DEFAULT 0 CHECK (resumed >= 0),
    favorites integer NOT NULL DEFAULT 0 CHECK (favorites >= 0),
    unmatched integer NOT NULL DEFAULT 0 CHECK (unmatched >= 0),
    last_problem text CHECK (last_problem IN ('unreachable', 'rate_limited', 'unexpected', 'profile_missing', 'sign_in_refused',
        'internal')),
    PRIMARY KEY (user_id, app)
);
