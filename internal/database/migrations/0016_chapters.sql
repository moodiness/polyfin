-- Analyses now keep the chapters of each version, which apps show and skip
-- through. Versions analyzed before, in a container that marks chapters
-- (Matroska, WebM, MP4, QuickTime), are analyzed again the next time one is
-- played: each on its own play, never all at once.
DELETE FROM media_analyses
WHERE analysis->>'format' LIKE 'matroska%' OR analysis->>'format' LIKE 'mov,%';
