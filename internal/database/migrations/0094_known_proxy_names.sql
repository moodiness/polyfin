-- The host names of the reverse proxies whose forwarded headers tell the
-- client of their requests, trusted at the addresses they resolve to, in
-- lower case; known_proxies and these hold at most 64 entries together.
ALTER TABLE settings
    ADD COLUMN known_proxy_names text[] NOT NULL DEFAULT '{}' CHECK (cardinality(known_proxy_names) <= 64);
