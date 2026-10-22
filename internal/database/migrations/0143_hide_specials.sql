-- Series' specials (season 0) are left out of seasons, episode lists and
-- Next Up; on by default, existing servers included.
ALTER TABLE settings ADD COLUMN hide_specials boolean NOT NULL DEFAULT true;
