-- A local folder's file is unmatched as no_search when no addon of the
-- server describes and searches titles of the folder's kind, so that
-- administrators see why, rather than not_found.
ALTER TABLE local_files DROP CONSTRAINT local_files_reason_check;
ALTER TABLE local_files ADD CONSTRAINT local_files_reason_check
    CHECK (reason IN ('', 'unreadable_name', 'not_found', 'ambiguous', 'other_year', 'search_failed', 'disc_title', 'disc_episode',
        'lone_extra', 'no_search'));
