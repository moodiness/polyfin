-- Only the subtitle tracks inside the files are offered, always: the
-- setting choosing it goes, and so do the two that only served other
-- subtitles. Addon subtitles are no longer synced, so no version's speech
-- is looked for, and picture subtitles are no longer read into text.
ALTER TABLE settings DROP COLUMN in_file_subtitles_only;
UPDATE settings SET sync_addon_subtitles = false, subtitle_ocr = 'off';
ALTER TABLE settings ALTER COLUMN sync_addon_subtitles SET DEFAULT false;
