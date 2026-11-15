-- The name a Stremio or Eclipse addon was given in Polyfin, shown instead
-- of its manifest's wherever Polyfin names it; NULL for its manifest's,
-- which it then follows (see addons.Store.SetName). A refresh of the
-- manifest keeps it. Polyfin's own sources keep their name in the manifest
-- Polyfin writes for them.
ALTER TABLE addons ADD COLUMN name text CHECK (name IS NULL OR length(name) BETWEEN 1 AND 64);
