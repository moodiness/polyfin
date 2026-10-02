-- The keyframes of a version's video, read from its container's index on
-- its first remux: where its HLS segments may start. Varints of the
-- microseconds between consecutive keyframes.
CREATE TABLE media_keyframes (
    version_id uuid PRIMARY KEY,
    keyframes bytea NOT NULL,
    indexed_at timestamptz NOT NULL DEFAULT now()
);
