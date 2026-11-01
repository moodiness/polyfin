-- The volume of a stereo mix becomes 2 by default, as in Jellyfin, with a
-- limiter after it. Servers still at the old default, 1, move to 2: 1 was
-- never told apart from a choice. Other values stay as they were.
ALTER TABLE settings
    ALTER COLUMN downmix_boost SET DEFAULT 2;

UPDATE settings SET downmix_boost = 2 WHERE downmix_boost = 1;
