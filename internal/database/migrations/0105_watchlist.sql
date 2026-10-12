-- The titles on each user's watchlist, movies and series, one row per list
-- that holds each: a tracking service's watchlist ('tracking:trakt'), an app
-- connection's library ('app:stremio'), or the one-off app imports
-- ('import'). A list's import replaces what it holds; the one-off imports
-- only add. added_at is when the list added the title, as it tells it, else
-- when Polyfin first found it there. The Watchlist view lists the titles
-- the user has not started, the latest added first (see package
-- watchlist).
CREATE TABLE watchlist (
    user_id uuid NOT NULL REFERENCES users ON DELETE CASCADE,
    item_id uuid NOT NULL,
    list text NOT NULL CHECK (list <> ''),
    added_at timestamptz NOT NULL,
    PRIMARY KEY (user_id, item_id, list)
);

-- Whether each user imports the watchlist of a tracking service they
-- connected (off by default), with the imports of its history, and how
-- many titles the last import added to their watchlist.
ALTER TABLE tracking_imports
    ADD COLUMN watchlist boolean NOT NULL DEFAULT false,
    ADD COLUMN watchlisted integer NOT NULL DEFAULT 0 CHECK (watchlisted >= 0);

-- An app connection's import puts the app's library on the watchlist rather
-- than in the favorites: its last import counts the titles it added there.
-- The favorites counted before were not put on the watchlist.
ALTER TABLE app_connections RENAME COLUMN favorites TO watchlist;
UPDATE app_connections SET watchlist = 0;
