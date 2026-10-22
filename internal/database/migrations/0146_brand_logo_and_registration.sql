-- The logo the web player shows in place of Jellyfin's, as a data URL of
-- the image; empty, the default, keeps Jellyfin's.
ALTER TABLE settings ADD COLUMN brand_logo text NOT NULL DEFAULT '';

-- Anyone who opens /register may create a member account with the default
-- settings, unless an administrator closes registration. Open by default.
ALTER TABLE settings ADD COLUMN registration boolean NOT NULL DEFAULT true;
