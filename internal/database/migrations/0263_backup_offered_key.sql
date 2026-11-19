-- An SFTP destination's host key is saved without being shown only at its
-- first connection. Once a key was forgotten, or the server's host or port
-- changed, host_key_forgotten is set: the next key offered is refused and
-- kept in offered_host_key, in the authorized_keys format, until an
-- administrator accepts it by its fingerprint. A saved key that no longer
-- matches keeps the key offered instead there too.
ALTER TABLE backup_destinations
    ADD COLUMN offered_host_key text NOT NULL DEFAULT '',
    ADD COLUMN host_key_forgotten boolean NOT NULL DEFAULT false;
