-- Lyrics an administrator chose among LRCLIB's search results for a music
-- track, in the web player's lyrics editor, or removed there: kept as the
-- lyrics LRCLIB answered are, and never replaced by an automatic look-up.
-- A removal is kept as lyrics of the kind 'removed', which has none.
ALTER TABLE track_lyrics
    ADD COLUMN chosen boolean NOT NULL DEFAULT false,
    DROP CONSTRAINT track_lyrics_kind_check,
    ADD CONSTRAINT track_lyrics_kind_check CHECK (kind IN ('synced', 'plain', 'instrumental', 'missing', 'removed'));
