-- The addresses and networks of the reverse proxies whose forwarded
-- headers tell the client of their requests, at most 64; none by default,
-- which trusts no forwarded header.
ALTER TABLE settings
    ADD COLUMN known_proxies inet[] NOT NULL DEFAULT '{}' CHECK (cardinality(known_proxies) <= 64);
