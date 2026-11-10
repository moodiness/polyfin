-- How the names of a movie's or an episode's versions are written: as the
-- addon writes them, or short, as Polyfin writes them from what it knows
-- of each version; and whether the web player shows the badges of the
-- version picked on a title's page and of the version played.
ALTER TABLE settings
    ADD COLUMN version_names text NOT NULL DEFAULT 'addon' CHECK (version_names IN ('addon', 'short')),
    ADD COLUMN version_badges boolean NOT NULL DEFAULT true;
