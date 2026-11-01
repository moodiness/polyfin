-- The series each user took off Next Up, with when. Such a series stays
-- off that user's Next Up until one of its episodes is played after it, a
-- new episode of it comes out after it, or the user puts it back; a
-- request for that one series still answers its next episode.
CREATE TABLE next_up_removed (
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    series_id uuid NOT NULL,
    removed_at timestamptz NOT NULL,
    PRIMARY KEY (user_id, series_id)
);
