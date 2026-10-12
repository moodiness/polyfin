-- Whether Polyfin answers the discovery messages Jellyfin apps broadcast on
-- the local network, on by default as Jellyfin's auto discovery is; and the
-- address it is reached at there, such as http://192.168.1.10:8096, which
-- the answers announce: empty, they announce the address of the interface
-- that reaches the app. An http or https URL without a trailing slash,
-- query or fragment.
ALTER TABLE settings
    ADD COLUMN local_discovery boolean NOT NULL DEFAULT true,
    ADD COLUMN local_address text NOT NULL DEFAULT ''
        CHECK (local_address = '' OR ((local_address LIKE 'http://%' OR local_address LIKE 'https://%') AND octet_length(local_address) <= 512));
