-- Where an addon subtitle timed on another cut of a title changes its
-- shift, at a scene the version adds or removes, when the version's speech
-- comes from a text track inside its file: a list of [start, end, offset]
-- triples in milliseconds, by start. The subtitle's lines that start from
-- start to end are of a scene the version lacks and are left out, and
-- those from end on are shifted by offset instead of offset_ms. Empty for
-- a subtitle with one shift.
ALTER TABLE subtitle_syncs
    ADD COLUMN cuts jsonb NOT NULL DEFAULT '[]';

-- The subtitles synced against a text track are timed again when next
-- chosen, now that their cuts are looked for.
DELETE FROM subtitle_syncs
WHERE version_id IN (SELECT version_id FROM version_speech WHERE reference = 'track');
