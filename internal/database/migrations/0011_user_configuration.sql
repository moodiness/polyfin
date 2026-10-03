-- The configuration each user saves from Jellyfin apps: playback preferences
-- (audio and subtitle languages, subtitle mode) and how libraries are
-- shown. The value is Polyfin's normalised form, not the app's request. A
-- user without a row has a new Jellyfin user's configuration.
CREATE TABLE user_configurations (
    user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    value jsonb NOT NULL CHECK (jsonb_typeof(value) = 'object')
);
