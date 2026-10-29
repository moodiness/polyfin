-- The proxy the server's addons' streams are read through, empty for
-- direct: a URL such as socks5h://warp:1080.
ALTER TABLE settings
    ADD COLUMN source_proxy text NOT NULL DEFAULT '';
