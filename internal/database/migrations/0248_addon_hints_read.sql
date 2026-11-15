-- Whether the configure hint of a Stremio or Eclipse addon's manifest
-- (behaviorHints.configurable) is stored. The manifests stored before
-- Polyfin kept it have it read once, in the background, at start (see
-- library.Service.ReadConfigureHints); a read that fails is tried again
-- at the next start. Installs, refreshes and new addresses store it at
-- once.
ALTER TABLE addons ADD COLUMN hints_read boolean NOT NULL DEFAULT true;
UPDATE addons SET hints_read = false WHERE kind IN ('stremio', 'eclipse');
