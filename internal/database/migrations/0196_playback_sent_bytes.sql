-- The bytes Polyfin sent the app for each video played, NULL when none
-- passed through it, as when the app read the source itself, and for the
-- videos played before it counted them.
ALTER TABLE playback_history
    ADD COLUMN sent_bytes bigint CHECK (sent_bytes > 0);
