-- Repackaged and converted plays of remote sources can start with fast
-- segments, 2 seconds each, converted quickly at 720p at most, where their
-- first segment is long. Off by default: off, plays start as they did.
ALTER TABLE settings
    ADD COLUMN fast_start boolean NOT NULL DEFAULT false;
