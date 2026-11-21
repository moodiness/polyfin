-- A folder in the container may follow the symbolic links leading out of
-- it, off by default; its last scan counts the links it skipped: leading
-- out of it, into a folder of the server one of its users may not reach,
-- to nothing or round in a loop, or to what Polyfin cannot read.
ALTER TABLE local_folders
    ADD COLUMN follow_links boolean NOT NULL DEFAULT false,
    ADD COLUMN links_out integer NOT NULL DEFAULT 0,
    ADD COLUMN links_fenced integer NOT NULL DEFAULT 0,
    ADD COLUMN links_broken integer NOT NULL DEFAULT 0,
    ADD COLUMN links_unreadable integer NOT NULL DEFAULT 0;
