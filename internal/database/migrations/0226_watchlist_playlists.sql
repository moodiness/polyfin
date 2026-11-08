-- Each user's watchlist becomes a playlist of theirs, flagged watchlist,
-- one per user, which every Jellyfin app lists with their playlists (see
-- package watchlist). Its entries are movies and episodes, as any
-- playlist's: a series stands for its aired episodes of regular seasons.
-- Its name is null while it keeps the default, which Polyfin gives in the
-- language of whoever sees it.
ALTER TABLE playlists
    ADD COLUMN IF NOT EXISTS watchlist boolean NOT NULL DEFAULT false,
    ALTER COLUMN name DROP NOT NULL;
ALTER TABLE playlists DROP CONSTRAINT IF EXISTS playlists_named;
ALTER TABLE playlists ADD CONSTRAINT playlists_named CHECK (watchlist OR name IS NOT NULL);
CREATE UNIQUE INDEX IF NOT EXISTS playlists_watchlist ON playlists (owner_id) WHERE watchlist;

-- A watchlist's entry names the title of watchlist it stands for: the
-- movie or the episode itself, or the series an episode came with; and an
-- episode's series. Other playlists' entries name neither.
ALTER TABLE playlist_entries
    ADD COLUMN IF NOT EXISTS title_id uuid,
    ADD COLUMN IF NOT EXISTS series_id uuid;
CREATE INDEX IF NOT EXISTS playlist_entries_item ON playlist_entries (playlist_id, item_id);
CREATE INDEX IF NOT EXISTS playlist_entries_title ON playlist_entries (playlist_id, title_id) WHERE title_id IS NOT NULL;

-- watchlist keeps what each list holds, as before. A title a list newly
-- holds waits here until its entries are made: at once for a movie, once
-- its episodes are known and aired for a series, in the background.
-- since is when it began to wait, tried_at when that was last tried:
-- hourly during its first day, then daily.
CREATE TABLE IF NOT EXISTS watchlist_pending (
    user_id uuid NOT NULL REFERENCES users ON DELETE CASCADE,
    item_id uuid NOT NULL,
    added_at timestamptz NOT NULL,
    since timestamptz NOT NULL DEFAULT now(),
    tried_at timestamptz,
    PRIMARY KEY (user_id, item_id)
);

-- Every user has their watchlist from the start, a new user too.
INSERT INTO playlists (owner_id, watchlist) SELECT id, true FROM users
ON CONFLICT (owner_id) WHERE watchlist DO NOTHING;

CREATE OR REPLACE FUNCTION watchlist_for_new_user() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO playlists (owner_id, watchlist) VALUES (NEW.id, true)
    ON CONFLICT (owner_id) WHERE watchlist DO NOTHING;
    RETURN NEW;
END
$$;
DROP TRIGGER IF EXISTS users_watchlist ON users;
CREATE TRIGGER users_watchlist AFTER INSERT ON users FOR EACH ROW EXECUTE FUNCTION watchlist_for_new_user();

-- What the Watchlist view showed, the titles not started, goes on: the
-- movies as entries, the latest added first; the others wait for their
-- episodes. The started titles stay in watchlist, as titles their lists
-- hold.
CREATE TEMPORARY TABLE watchlist_shown ON COMMIT DROP AS
SELECT w.user_id, w.item_id, max(w.added_at) AS added_at, i.kind
FROM watchlist w LEFT JOIN items i ON i.id = w.item_id
WHERE NOT EXISTS (SELECT 1 FROM user_data d WHERE d.user_id = w.user_id
    AND (d.item_id = w.item_id OR d.series_id = w.item_id) AND (d.played OR d.position_ticks > 0))
GROUP BY w.user_id, w.item_id, i.kind;

INSERT INTO playlist_entries (playlist_id, item_id, title_id, position)
SELECT p.id, s.item_id, s.item_id,
    (SELECT count(*) FROM playlist_entries e WHERE e.playlist_id = p.id)
        + row_number() OVER (PARTITION BY s.user_id ORDER BY s.added_at DESC, s.item_id) - 1
FROM watchlist_shown s JOIN playlists p ON p.owner_id = s.user_id AND p.watchlist
WHERE s.kind = 'movie'
    AND NOT EXISTS (SELECT 1 FROM playlist_entries e WHERE e.playlist_id = p.id AND e.item_id = s.item_id);

INSERT INTO watchlist_pending (user_id, item_id, added_at)
SELECT s.user_id, s.item_id, s.added_at FROM watchlist_shown s
WHERE s.kind IS DISTINCT FROM 'movie'
    AND NOT EXISTS (SELECT 1 FROM playlists p JOIN playlist_entries e ON e.playlist_id = p.id
        WHERE p.owner_id = s.user_id AND p.watchlist AND e.title_id = s.item_id)
ON CONFLICT (user_id, item_id) DO NOTHING;

UPDATE playlists p SET last_added_at = (SELECT max(w.added_at) FROM watchlist w WHERE w.user_id = p.owner_id)
WHERE p.watchlist AND p.last_added_at IS NULL AND EXISTS (SELECT 1 FROM playlist_entries e WHERE e.playlist_id = p.id);

-- The Watchlist view goes, and its images with it. Each user's image of a
-- playlist goes with it.
ALTER TABLE personal_images ADD COLUMN IF NOT EXISTS playlist_id uuid REFERENCES playlists ON DELETE CASCADE;
DELETE FROM personal_images WHERE smart_list_id IS NULL AND playlist_id IS NULL;
ALTER TABLE personal_images DROP CONSTRAINT IF EXISTS personal_images_source;
ALTER TABLE personal_images ADD CONSTRAINT personal_images_source CHECK ((smart_list_id IS NULL) <> (playlist_id IS NULL));
CREATE INDEX IF NOT EXISTS personal_images_playlist ON personal_images (playlist_id) WHERE playlist_id IS NOT NULL;
