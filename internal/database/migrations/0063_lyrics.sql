-- Song lyrics from LRCLIB, on by default: whether music tracks are looked
-- up there and their lyrics served to apps.
ALTER TABLE settings
    ADD COLUMN lyrics boolean NOT NULL DEFAULT true;

-- What LRCLIB answered for each music track, by the track's item, so that
-- each track is asked about once: its synced lyrics (LRC text) or plain
-- lyrics when it has them, that it is an instrumental, or that LRCLIB does
-- not know it, which is asked again once looked_up_at is old enough. A
-- request that failed leaves no row: the track is asked again later.
CREATE TABLE track_lyrics (
    item_id uuid PRIMARY KEY,
    kind text NOT NULL CHECK (kind IN ('synced', 'plain', 'instrumental', 'missing')),
    lyrics text NOT NULL DEFAULT '',
    looked_up_at timestamptz NOT NULL DEFAULT now()
);
