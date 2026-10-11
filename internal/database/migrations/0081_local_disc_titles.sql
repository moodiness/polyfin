-- A Blu-ray disc holding several movies lists each as a title of its own;
-- one other than the disc's main title is matched only by a link, its
-- reason being disc_title until then.
ALTER TABLE local_files DROP CONSTRAINT local_files_reason_check;
ALTER TABLE local_files ADD CONSTRAINT local_files_reason_check
    CHECK (reason IN ('', 'unreadable_name', 'not_found', 'ambiguous', 'other_year', 'search_failed', 'disc_title'));
