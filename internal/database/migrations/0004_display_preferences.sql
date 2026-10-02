-- Display preferences Jellyfin apps save per user, preference id and client
-- app: how a library or item is sorted and shown, and the app's own
-- settings. The value is Polyfin's normalised form, not the app's request.
CREATE TABLE display_preferences (
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    preference_id text NOT NULL,
    client text NOT NULL,
    value jsonb NOT NULL CHECK (jsonb_typeof(value) = 'object'),
    PRIMARY KEY (user_id, preference_id, client)
);
