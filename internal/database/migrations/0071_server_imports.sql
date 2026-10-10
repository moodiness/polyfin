-- Whether users may import their own watch history from another Jellyfin
-- or Emby server, under My account.
ALTER TABLE settings
    ADD COLUMN server_imports boolean NOT NULL DEFAULT true;
