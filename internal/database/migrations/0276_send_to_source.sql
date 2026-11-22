-- Send apps to the source becomes a choice of three: the source's owners
-- (an administrator for the server's addons, a user for their own, no one
-- for IPTV sources), the default and what off was; everyone, what on was;
-- or no one, every play relayed through the server, administrators' and
-- owners' included.
ALTER TABLE settings
    ADD COLUMN send_to_source text NOT NULL DEFAULT 'owners' CHECK (send_to_source IN ('owners', 'everyone', 'none'));
UPDATE settings SET send_to_source = 'everyone' WHERE send_apps_to_source;
ALTER TABLE settings
    DROP COLUMN send_apps_to_source;
