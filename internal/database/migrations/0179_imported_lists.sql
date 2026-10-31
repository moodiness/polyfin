-- The collections and playlists imports brought from Jellyfin, Emby and
-- Plex servers, by the server, its kind and its own identifier, and the
-- list's identifier there, so that importing again adds nothing twice.
-- A list deleted in Polyfin loses its row, and the next import brings it
-- again.
CREATE TABLE imported_collections (
    server text NOT NULL,
    source_id text NOT NULL,
    collection_id uuid NOT NULL REFERENCES collections ON DELETE CASCADE,
    -- The user whose import brought it, NULL once they are deleted.
    imported_by uuid REFERENCES users ON DELETE SET NULL,
    imported_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (server, source_id)
);
CREATE INDEX imported_collections_collection ON imported_collections (collection_id);

-- A playlist is brought into the account of each user who sees it on the
-- server, as their own.
CREATE TABLE imported_playlists (
    server text NOT NULL,
    source_id text NOT NULL,
    user_id uuid NOT NULL REFERENCES users ON DELETE CASCADE,
    playlist_id uuid NOT NULL REFERENCES playlists ON DELETE CASCADE,
    imported_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (server, source_id, user_id)
);
CREATE INDEX imported_playlists_playlist ON imported_playlists (playlist_id);
