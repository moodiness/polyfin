-- Restart from the admin app: whether an administrator confirmed that
-- Polyfin is started again when it exits, which Polyfin cannot see in a
-- container without the Docker socket. Update now: the address of the
-- update API of a tool the administrator runs beside Polyfin, and the
-- bearer token sent to it, sealed with POLYFIN_SECRET_KEY as the other
-- stored secrets.
ALTER TABLE settings
    ADD COLUMN restart_confirmed boolean NOT NULL DEFAULT false,
    ADD COLUMN update_tool_url text NOT NULL DEFAULT '',
    ADD COLUMN update_tool_token text NOT NULL DEFAULT '';
