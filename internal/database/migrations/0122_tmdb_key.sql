-- The server's TMDB key, which Edit images searches TMDB's images with,
-- empty for none: an API key or a read access token, sealed as the
-- server's other keys are, which makes it longer than the key itself.
ALTER TABLE settings
    ADD COLUMN tmdb_key text NOT NULL DEFAULT '' CHECK (octet_length(tmdb_key) <= 1024 AND tmdb_key ~ '^[ -~]*$');
