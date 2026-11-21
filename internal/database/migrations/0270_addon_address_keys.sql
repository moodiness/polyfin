-- With POLYFIN_SECRET_KEY set, the address of a Stremio or Eclipse addon,
-- of an M3U list or of an Xtream account, which embeds its keys or its
-- password, is stored encrypted, under a nonce of its own (see the secrets
-- package): two encryptions of one address differ. address_key then holds
-- a keyed hash of the address, which tells the same address installed
-- twice in a scope without telling the address. It is NULL for an address
-- stored as it is: without the key, a local folder's path and a tuner's
-- address, which hold no credentials.
ALTER TABLE addons ADD COLUMN address_key text CHECK (address_key <> '');

DROP INDEX addons_unique_url;
CREATE UNIQUE INDEX addons_unique_url
    ON addons (coalesce(owner_id, '00000000-0000-0000-0000-000000000000'), coalesce(address_key, manifest_url));

-- A guide's address is encrypted too, which makes it longer: Polyfin
-- bounds the address itself, before it is encrypted.
ALTER TABLE live_guides
    DROP CONSTRAINT live_guides_url_check,
    ADD CONSTRAINT live_guides_url_check CHECK (url <> '');
