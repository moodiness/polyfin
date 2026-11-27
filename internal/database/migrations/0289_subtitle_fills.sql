-- The FFmpeg indexes of the subtitle streams a version's index lists
-- block by block but that its host, serving one range per request, would
-- take too many requests to serve: they are offered once a fill has read
-- their span of the file into the source cache, and they are read from
-- there. Empty when there are none. NULL for the indexes kept before,
-- which left such streams out of located: an index that does not locate
-- every subtitle stream it may is read again.
ALTER TABLE media_subtitle_index ADD COLUMN awaiting integer[];
