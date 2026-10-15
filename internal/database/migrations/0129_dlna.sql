-- Sharing the libraries over DLNA (see package dlna), off by default: the
-- name devices show for the server, empty for the server's name; the user
-- whose libraries a device seen for the first time shows, none by default,
-- which gives it an empty server; and whether a title a device reads past
-- Marked played after (%) is marked played, off by default.
ALTER TABLE settings
    ADD COLUMN dlna boolean NOT NULL DEFAULT false,
    ADD COLUMN dlna_name text NOT NULL DEFAULT '' CHECK (octet_length(dlna_name) <= 256),
    ADD COLUMN dlna_new_devices_user uuid REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN dlna_mark_played boolean NOT NULL DEFAULT false;

-- The device profiles an administrator added, in the XML format of
-- Jellyfin's DLNA plugin, beside the built-in ones.
CREATE TABLE dlna_profiles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 128),
    xml text NOT NULL CHECK (octet_length(xml) <= 262144),
    created_at timestamptz NOT NULL DEFAULT now()
);

-- The DLNA devices seen on the local network, one per address: the name,
-- manufacturer and model they announce, and the headers they last sent.
-- A device shows the libraries of its user, none while user_id is null,
-- and nothing while it is blocked. profile forces a device profile: a
-- built-in one by its name, prefixed with "builtin:", or an added one by
-- its ID; empty, the profile is chosen from what the device sends.
CREATE TABLE dlna_devices (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    address text NOT NULL UNIQUE CHECK (octet_length(address) <= 64),
    name text NOT NULL DEFAULT '' CHECK (octet_length(name) <= 256),
    manufacturer text NOT NULL DEFAULT '' CHECK (octet_length(manufacturer) <= 256),
    model text NOT NULL DEFAULT '' CHECK (octet_length(model) <= 256),
    user_agent text NOT NULL DEFAULT '' CHECK (octet_length(user_agent) <= 512),
    client_info text NOT NULL DEFAULT '' CHECK (octet_length(client_info) <= 512),
    user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    blocked boolean NOT NULL DEFAULT false,
    profile text NOT NULL DEFAULT '' CHECK (octet_length(profile) <= 160),
    first_seen timestamptz NOT NULL DEFAULT now(),
    last_seen timestamptz NOT NULL DEFAULT now()
);
