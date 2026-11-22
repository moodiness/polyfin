-- When a user last chose Reset my preferences, NULL if never. Polyfin's
-- script in jellyfin-web reads it, and forgets the settings a browser
-- keeps for the user when it is newer than the reset the browser applied.
ALTER TABLE users ADD COLUMN preferences_reset_at timestamptz;
