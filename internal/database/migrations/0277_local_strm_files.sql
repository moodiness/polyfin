-- The address a .strm file of a movies or shows folder names, which its
-- version plays as an addon's link is played: NULL for a video the
-- folder holds. A scan reads it when the file is new or changed. With
-- POLYFIN_SECRET_KEY set, it is stored encrypted, as an addon's address
-- is (see the secrets package): it may hold a key. No query looks a file
-- up by it, so it needs no keyed hash.
ALTER TABLE local_files
    ADD COLUMN stream_url text CHECK (stream_url <> '');
