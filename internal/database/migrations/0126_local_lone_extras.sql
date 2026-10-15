-- A video named as an extra, such as "Movie-trailer.mkv", whose movie is
-- not beside it is unmatched, its reason lone_extra, so that an
-- administrator sees it and may link it: a link makes it a title.
ALTER TABLE local_files DROP CONSTRAINT local_files_reason_check;
ALTER TABLE local_files ADD CONSTRAINT local_files_reason_check
    CHECK (reason IN ('', 'unreadable_name', 'not_found', 'ambiguous', 'other_year', 'search_failed', 'disc_title', 'disc_episode',
        'lone_extra'));
