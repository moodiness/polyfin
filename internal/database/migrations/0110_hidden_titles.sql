-- When the user last took the item off Continue Watching: its resume point
-- was cleared then, and an import brings back only a resume point set
-- after it.
ALTER TABLE user_data ADD COLUMN resume_removed_at timestamptz;

-- The movies and series each user hid from their lists, with when. A
-- hidden title leaves that user's listings, home rows, searches and similar
-- titles, a series with its episodes; it still opens by its identifier.
CREATE TABLE hidden_titles (
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    item_id uuid NOT NULL,
    hidden_at timestamptz NOT NULL,
    PRIMARY KEY (user_id, item_id)
);
