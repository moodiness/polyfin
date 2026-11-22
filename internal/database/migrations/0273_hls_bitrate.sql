-- Analyses of HLS playlists now keep the bitrate of the variant Polyfin
-- plays, and no size, where they kept the playlist's own bytes and the
-- bitrate those make. Versions analyzed before are analyzed again the next
-- time one is played: each on its own play, never all at once.
DELETE FROM media_analyses WHERE analysis->>'format' = 'hls';
