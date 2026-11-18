-- network tells where a source is: 'local' for one Polyfin read from a
-- local address (loopback, private, link-local, unique local,
-- carrier-grade NAT), from a file of the server, or a local folder;
-- 'public' for one it read from an address on the internet. It is what
-- Polyfin connected to when it last read the source's manifest or list
-- (see addons.Network), set when the source is installed, when its
-- address changes and when it is read again. NULL is not told yet: the
-- sources installed before are told at start (see
-- addons.Store.LocateSources), and count as public until then.
--
-- Only a source on a local network, the server's or an administrator's
-- own, may name addresses of the local network: the requests for what
-- any other names stay on public addresses (see addons.Addon.Confined).
ALTER TABLE addons ADD COLUMN network text CHECK (network IN ('local', 'public'));

-- A local folder is on the server itself.
UPDATE addons SET network = 'local' WHERE kind = 'local';

-- A person's record does not tell which source gave their photo: the
-- photos addons gave before are downloaded from public addresses only,
-- until a title's description is read again and records its people by
-- the rule.
UPDATE items SET data = data || '{"confined": true}'
    WHERE kind = 'person' AND data->'person' ? 'image' AND NOT coalesce((data->>'confined')::boolean, false);
