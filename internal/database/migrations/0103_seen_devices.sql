-- The device identifiers each account signed in from, through a Jellyfin
-- app or the web client, when first and last, kept after the device signs
-- out: a sign-in from one the account never used is told to the user. The
-- devices signed in at the migration count as seen, so that updating tells
-- nothing.
CREATE TABLE seen_devices (
    user_id uuid NOT NULL REFERENCES users ON DELETE CASCADE,
    device_id text NOT NULL,
    first_seen_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, device_id)
);

INSERT INTO seen_devices (user_id, device_id) SELECT DISTINCT user_id, device_id FROM devices;
