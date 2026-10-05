-- With POLYFIN_SECRET_KEY set, the server's keys are stored sealed
-- ("enc:v1:" and base64), longer than the keys themselves: their checks
-- only bound the size now, and keep them to printable ASCII, which sealed
-- values are too. Polyfin checks the keys themselves before sealing them.
ALTER TABLE settings
    DROP CONSTRAINT settings_publicmetadb_key_check,
    DROP CONSTRAINT settings_theintrodb_key_check,
    DROP CONSTRAINT settings_trakt_client_secret_check,
    ADD CONSTRAINT settings_publicmetadb_key_check CHECK (octet_length(publicmetadb_key) <= 512 AND publicmetadb_key ~ '^[ -~]*$'),
    ADD CONSTRAINT settings_theintrodb_key_check CHECK (octet_length(theintrodb_key) <= 512 AND theintrodb_key ~ '^[ -~]*$'),
    ADD CONSTRAINT settings_trakt_client_secret_check CHECK (octet_length(trakt_client_secret) <= 512 AND trakt_client_secret ~ '^[!-~]*$');
