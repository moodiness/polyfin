-- Whether a server Stremio addon's streams are read through the source
-- proxy set in Settings › Playback, off until an administrator turns it on.
ALTER TABLE addons
    ADD COLUMN use_source_proxy boolean NOT NULL DEFAULT false;
