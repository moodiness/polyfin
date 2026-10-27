-- An admin session opened with a Jellyfin app's token, as the admin app
-- does with jellyfin-web's, names the device it came from. It ends with
-- that device's sign-in: signing out, a sign-out from Devices, or another
-- sign-in on the device. Sessions opened with a password name none.
ALTER TABLE admin_sessions
    ADD COLUMN device uuid REFERENCES devices (id) ON DELETE CASCADE;
CREATE INDEX admin_sessions_device ON admin_sessions (device) WHERE device IS NOT NULL;

-- The sessions opened before name no device, and nothing tells those
-- taken from a web client, which could outlive its sign-out, from those
-- opened with a password: all end. Everyone signs in to the admin app
-- once; arriving from the web client does it by itself.
DELETE FROM admin_sessions;
