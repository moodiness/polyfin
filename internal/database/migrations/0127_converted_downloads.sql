-- Converted downloads: an app whose device profile asks for video streamed
-- as one file, as apps saving a title for offline viewing do, gets it
-- copied or converted within the user's quality. Off by default.
ALTER TABLE settings ADD COLUMN converted_downloads boolean NOT NULL DEFAULT false;
