-- Polyfin names itself to plex.tv and to Plex Media Servers by this client
-- identifier, so that each sign-in with Plex reuses one entry among the
-- account's devices. It is generated once, as the server's identity is.
ALTER TABLE server_identity ADD COLUMN plex_client uuid NOT NULL DEFAULT gen_random_uuid();
