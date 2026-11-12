-- A local folder's file whose container has no index now gets its images,
-- FFmpeg reading it whole: the results telling that such a file cannot
-- have them are dropped, so that the task Make thumbnails makes them.
DELETE FROM thumbnail_results WHERE kind = 'local' AND reason = 'no_index';
