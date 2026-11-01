-- The audio of the videos the server converts can be evened at -16 LUFS,
-- in one pass. Off by default: off, converted audio keeps its level, and
-- direct play, copied audio, music and audiobooks never change.
ALTER TABLE settings
    ADD COLUMN normalize_converted_audio boolean NOT NULL DEFAULT false;
