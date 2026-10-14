-- Share links: a member allowed to share gives one movie or one episode to
-- someone without an account, through a link that expires. share_links
-- turns them on for the server, off by default: off, none is made and
-- every one is refused, without being deleted. can_share lets a user make
-- them, off by default for every user, administrators included.
ALTER TABLE settings
    ADD COLUMN share_links boolean NOT NULL DEFAULT false;

ALTER TABLE users
    ADD COLUMN can_share boolean NOT NULL DEFAULT false;

-- A share link of user_id's: the title it opens, its kind and its name
-- as it was when the link was made; the hash of its token's secret; how
-- many guests may watch at once; when it expires, and when it was revoked.
-- plays counts the playbacks it started. expiry_recorded is set once its
-- expiry is in the activity log; notified_at is when share_played last
-- told of it, at most once an hour.
CREATE TABLE share_links (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    item_id uuid NOT NULL,
    kind text NOT NULL CHECK (kind IN ('movie', 'episode')),
    title text NOT NULL CHECK (octet_length(title) <= 1024),
    secret_hash bytea NOT NULL CHECK (length(secret_hash) = 32),
    max_viewers integer NOT NULL CHECK (max_viewers BETWEEN 1 AND 5),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    plays integer NOT NULL DEFAULT 0 CHECK (plays >= 0),
    expiry_recorded boolean NOT NULL DEFAULT false,
    notified_at timestamptz
);

CREATE INDEX share_links_user ON share_links (user_id, created_at DESC);
CREATE INDEX share_links_expiring ON share_links (expires_at) WHERE NOT expiry_recorded;

-- The playbacks of a guest through a share link count as the sharer's,
-- marked as shared.
ALTER TABLE playback_history
    ADD COLUMN shared boolean NOT NULL DEFAULT false;
