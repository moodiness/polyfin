-- A file left unmatched as its title's search failed tells the addon that
-- failed it and why, as the stremio package names failures; empty for the
-- others.
ALTER TABLE local_files
    ADD COLUMN search_addon text NOT NULL DEFAULT '',
    ADD COLUMN search_failure text NOT NULL DEFAULT '';

-- A folder some of whose titles' searches failed is scanned again at
-- search_retry_at, later after each failure in a row, which search_retries
-- counts; NULL and 0 once its searches went through.
ALTER TABLE local_folders
    ADD COLUMN search_retry_at timestamptz,
    ADD COLUMN search_retries integer NOT NULL DEFAULT 0 CHECK (search_retries >= 0);
