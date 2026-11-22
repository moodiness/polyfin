-- How many minutes of each watched Live TV channel are kept on disk for
-- pause and rewind (0 for none), and how much disk space, in GB, the
-- buffers of all channels may take together.
ALTER TABLE settings
    ADD COLUMN live_tv_buffer_minutes integer NOT NULL DEFAULT 30 CHECK (live_tv_buffer_minutes BETWEEN 0 AND 240),
    ADD COLUMN live_tv_buffer_gigabytes integer NOT NULL DEFAULT 20 CHECK (live_tv_buffer_gigabytes BETWEEN 1 AND 1000);
