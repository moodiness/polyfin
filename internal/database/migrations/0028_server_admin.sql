-- API keys tools and apps use the Jellyfin API with, with administrator
-- rights and no user. Only a SHA-256 hash of each key is stored.
CREATE TABLE api_keys (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    app text NOT NULL CHECK (length(app) BETWEEN 1 AND 64),
    token_hash bytea NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz
);

-- The names administrators give Jellyfin apps' devices, by the device
-- identifier the apps send, as Jellyfin's DeviceOptions.
CREATE TABLE device_options (
    device_id text PRIMARY KEY CHECK (length(device_id) BETWEEN 1 AND 256),
    id integer GENERATED ALWAYS AS IDENTITY UNIQUE,
    custom_name text CHECK (length(custom_name) <= 256)
);

-- What happened on the server, as Jellyfin's activity log: sign-ins,
-- playback, changes to users, settings and addons. Entries are kept 30
-- days. user_id names the user an entry is about, kept after the user is
-- deleted.
CREATE TABLE activity_log (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 1024),
    overview text CHECK (length(overview) <= 4096),
    short_overview text CHECK (length(short_overview) <= 1024),
    type text NOT NULL CHECK (length(type) BETWEEN 1 AND 64),
    item_id text CHECK (length(item_id) <= 64),
    user_id uuid,
    date timestamptz NOT NULL DEFAULT now(),
    severity text NOT NULL DEFAULT 'Information' CHECK (severity IN ('Information', 'Warning', 'Error'))
);
CREATE INDEX activity_log_date ON activity_log (date DESC, id DESC);

-- The PIN a user asked for from a Jellyfin app's forgotten password
-- screen, one per user, valid until expires_at. Like Jellyfin's PIN file,
-- the PIN is kept as is: the administrator reads it to the user.
CREATE TABLE password_reset_pins (
    user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    pin text NOT NULL CHECK (length(pin) BETWEEN 8 AND 16),
    expires_at timestamptz NOT NULL
);
