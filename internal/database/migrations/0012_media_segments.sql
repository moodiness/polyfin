-- What each community database answered about the segments of a title (a
-- movie or an episode) that apps offer to skip: per database, the segments
-- found and when to ask again, so that the databases are asked about a
-- title only once their answer expires.
CREATE TABLE media_segments (
    item_id uuid PRIMARY KEY,
    answers jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
