-- Moonfin's companion (see package moonfin): Polyfin answers the routes
-- Moonfin apps call on a server for their extra features, settings synced
-- across a user's devices first.
--
-- settings.moonfin serves the companion to users by default; users.moonfin
-- sets a user apart, 'on' or 'off', empty for the server's default, and
-- users.moonfin_locked keeps the user from changing it themselves.
ALTER TABLE settings ADD COLUMN moonfin boolean NOT NULL DEFAULT true;

ALTER TABLE users
    ADD COLUMN moonfin text NOT NULL DEFAULT '' CHECK (moonfin IN ('', 'on', 'off')),
    ADD COLUMN moonfin_locked boolean NOT NULL DEFAULT false;

-- Each user's Moonfin settings, as their apps sync them: one JSON object
-- holding the settings every device shares (global) and those of desktop,
-- mobile and TV devices, kept as the apps send them. baselines keeps, for
-- each kind of device, the titles it was last told were hidden from
-- Continue Watching and Next Up, which tells a title a device brought back
-- from one it never heard of.
CREATE TABLE moonfin_settings (
    user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    envelope jsonb NOT NULL CHECK (jsonb_typeof(envelope) = 'object'),
    baselines jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(baselines) = 'object'),
    updated_at timestamptz NOT NULL
);

-- The administrator's defaults for Moonfin's settings, which apps take for
-- the settings a user never set: those of every device (global), and those
-- of one kind of device, which win over them there.
CREATE TABLE moonfin_defaults (
    profile text PRIMARY KEY CHECK (profile IN ('global', 'desktop', 'mobile', 'tv')),
    settings jsonb NOT NULL CHECK (jsonb_typeof(settings) = 'object')
);
