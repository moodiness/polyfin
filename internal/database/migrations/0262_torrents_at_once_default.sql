-- A new server runs 10 torrents at once at most, server-wide. A server is
-- new while it has no account: the row of settings it was given by the
-- first migration takes the new default. A server set up before keeps its
-- value, 0 for no limit unless its administrator chose one.
ALTER TABLE settings ALTER COLUMN torrents_at_once SET DEFAULT 10;
UPDATE settings SET torrents_at_once = DEFAULT
    WHERE torrents_at_once = 0 AND NOT EXISTS (SELECT 1 FROM users);
