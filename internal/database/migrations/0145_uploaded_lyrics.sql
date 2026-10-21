-- Lyrics an administrator uploaded for a music track in the web player's
-- lyrics editor: an administrator's choice, as lyrics chosen among
-- LRCLIB's results are, but not LRCLIB's, so that they are served whether
-- Song lyrics from LRCLIB is on or not.
ALTER TABLE track_lyrics
    ADD COLUMN uploaded boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT track_lyrics_uploaded_chosen CHECK (chosen OR NOT uploaded);
