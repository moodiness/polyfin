-- Where an entry's name names its item, the title of a playback or a
-- share link, in characters from the start of the name: the admin app's
-- Recent activity links those words to the title's page. Null for the
-- other entries, and for those written before.
ALTER TABLE activity_log
    ADD COLUMN title_start integer,
    ADD COLUMN title_end integer,
    ADD CONSTRAINT activity_log_title CHECK (
        (title_start IS NULL) = (title_end IS NULL)
        AND (title_start IS NULL OR item_id IS NOT NULL)
        AND title_start >= 0 AND title_start < title_end AND title_end <= length(name)
    );
