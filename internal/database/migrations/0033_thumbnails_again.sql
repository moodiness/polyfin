-- Thumbnails made by Polyfin 0.7.0 may show their keyframes out of order:
-- one decoder fed every keyframe of an HEVC version could output them in
-- another order than they were read. Every thumbnail and chapter image is
-- dropped, so that the next play of each version makes them again. Their
-- tiles and images go with their versions.
DELETE FROM thumbnail_versions;
