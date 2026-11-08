-- A home row may list the user's Watchlist (watchlist), one row for each
-- playlist the user made a favorite (playlists), or a playlist the
-- administrator chose (playlist, playlist_id), whose deletion deletes the
-- row. The favorites' rows are never a library in other apps.
ALTER TABLE home_rows
    DROP CONSTRAINT home_rows_kind_check,
    ADD CONSTRAINT home_rows_kind_check CHECK (kind IN ('catalog', 'collection', 'because', 'new', 'available', 'waiting', 'trakt', 'picks',
        'watchlist', 'playlists', 'playlist')),
    ADD COLUMN playlist_id uuid REFERENCES playlists (id) ON DELETE CASCADE,
    ADD CONSTRAINT home_rows_playlist_check CHECK ((kind = 'playlist') = (playlist_id IS NOT NULL)),
    ADD CONSTRAINT home_rows_playlists_library_check CHECK (kind <> 'playlists' OR NOT library);
CREATE INDEX home_rows_playlist ON home_rows (playlist_id) WHERE playlist_id IS NOT NULL;
