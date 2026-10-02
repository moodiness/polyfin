-- The secret signing the playback URLs and play sessions Polyfin hands to
-- players: 32 bytes from two random UUIDs (244 random bits).
ALTER TABLE server_identity ADD COLUMN secret bytea NOT NULL
    DEFAULT decode(replace(gen_random_uuid()::text, '-', '') || replace(gen_random_uuid()::text, '-', ''), 'hex');

-- What ffprobe found in a version of a title, analyzed on its first
-- playback.
CREATE TABLE media_analyses (
    version_id uuid PRIMARY KEY,
    analysis jsonb NOT NULL,
    analyzed_at timestamptz NOT NULL DEFAULT now()
);
