-- Playlists users make of movies and episodes. The owner edits a playlist;
-- open access lets every user see it.
CREATE TABLE playlists (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name text NOT NULL,
    open_access boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    -- When titles were last added, NULL before any.
    last_added_at timestamptz
);
CREATE INDEX playlists_owner ON playlists (owner_id);

-- The users a playlist is shared with, in the order they were added; those
-- who can edit change it as its owner does.
CREATE TABLE playlist_shares (
    playlist_id uuid NOT NULL REFERENCES playlists (id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    can_edit boolean NOT NULL DEFAULT false,
    ordinal bigint GENERATED ALWAYS AS IDENTITY,
    PRIMARY KEY (playlist_id, user_id)
);
CREATE INDEX playlist_shares_user ON playlist_shares (user_id);

-- The titles of a playlist, from position 0. Each entry has an identifier of
-- its own, which apps move and remove it by, so that a title can appear more
-- than once. Positions are checked at commit, so that a reorder can shift
-- them in one statement.
CREATE TABLE playlist_entries (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    playlist_id uuid NOT NULL REFERENCES playlists (id) ON DELETE CASCADE,
    item_id uuid NOT NULL,
    position integer NOT NULL CHECK (position >= 0),
    UNIQUE (playlist_id, position) DEFERRABLE INITIALLY DEFERRED
);
