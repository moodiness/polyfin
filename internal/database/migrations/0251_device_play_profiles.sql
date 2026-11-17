-- How each signed-in device told it plays, kept across restarts so that
-- item details name versions with how they play from the start (see
-- jellyfin's devicePlay): the device profile of its last PlaybackInfo and
-- the bitrate it asked for, null when it sent none, else the profile its
-- session's capabilities carried (from_capabilities). A profile goes with
-- its device, signed out or deleted with its user.
CREATE TABLE device_play_profiles (
    device_id uuid PRIMARY KEY REFERENCES devices (id) ON DELETE CASCADE,
    profile jsonb NOT NULL CHECK (jsonb_typeof(profile) = 'object' AND octet_length(profile::text) <= 131072),
    bitrate bigint,
    from_capabilities boolean NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
