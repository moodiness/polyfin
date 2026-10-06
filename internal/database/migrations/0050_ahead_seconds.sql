-- How far a remux or conversion of a file runs ahead of the player counts
-- seconds of picture rather than segments, whose length now varies (2 s
-- for the first, about 4 s after). A server left at the former default of
-- 10 segments takes the new default, 120 s; another value keeps its length
-- at about 6 s a segment, within the new bounds.
ALTER TABLE settings
    ADD COLUMN ahead_seconds integer NOT NULL DEFAULT 120 CHECK (ahead_seconds BETWEEN 30 AND 600);

UPDATE settings
SET ahead_seconds = CASE WHEN ahead_segments = 10 THEN 120 ELSE LEAST(GREATEST(ahead_segments * 6, 30), 600) END;

ALTER TABLE settings DROP COLUMN ahead_segments;
