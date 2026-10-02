-- Accounts shared by Jellyfin apps and the admin interface. Names are unique
-- without regard to case, as Jellyfin apps sign in case-insensitively.
CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 64),
    password_hash text NOT NULL,
    is_administrator boolean NOT NULL DEFAULT false,
    is_hidden boolean NOT NULL DEFAULT true,
    is_disabled boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_login_at timestamptz,
    last_activity_at timestamptz
);
CREATE UNIQUE INDEX users_name_unique ON users (lower(name));

-- One signed-in Jellyfin device per user and device identifier: signing in
-- again from the same device replaces its token. Only a SHA-256 hash of the
-- access token is stored.
CREATE TABLE devices (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash bytea NOT NULL UNIQUE,
    device_id text NOT NULL,
    device_name text NOT NULL,
    client text NOT NULL,
    client_version text NOT NULL,
    remote_address text NOT NULL,
    capabilities jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(capabilities) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    last_activity_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, device_id)
);

-- Browser sessions of the admin interface, also stored as token hashes.
CREATE TABLE admin_sessions (
    token_hash bytea PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);
CREATE INDEX admin_sessions_user ON admin_sessions (user_id);

CREATE TABLE settings (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    server_name text NOT NULL DEFAULT 'Polyfin' CHECK (length(server_name) BETWEEN 1 AND 64),
    quick_connect_enabled boolean NOT NULL DEFAULT true,
    legacy_authorization boolean NOT NULL DEFAULT false
);
INSERT INTO settings DEFAULT VALUES;
