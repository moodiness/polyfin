-- Whether the server converts (re-encodes) video and audio for apps that
-- cannot play a file as it is, and whether it lets users download titles.
ALTER TABLE settings
    ADD COLUMN transcoding boolean NOT NULL DEFAULT true,
    ADD COLUMN downloads boolean NOT NULL DEFAULT true;

-- Each user's own permissions, as Jellyfin's user policy holds them: video
-- conversion (burning subtitles in included), audio conversion, and
-- downloads. Like Jellyfin's, they are granted unless taken away.
ALTER TABLE users
    ADD COLUMN video_transcoding boolean NOT NULL DEFAULT true,
    ADD COLUMN audio_transcoding boolean NOT NULL DEFAULT true,
    ADD COLUMN content_downloading boolean NOT NULL DEFAULT true;
