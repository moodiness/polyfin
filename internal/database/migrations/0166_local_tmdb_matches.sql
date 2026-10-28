-- A local folder's file whose title every metadata addon failed to search
-- may be matched through TMDB's search, while the server has a TMDB key:
-- matched_by "tmdb" (see localfiles.searchFallback).
ALTER TABLE local_files
    DROP CONSTRAINT local_files_matched_by_check,
    ADD CONSTRAINT local_files_matched_by_check
        CHECK (matched_by IN ('', 'name', 'nfo', 'search', 'year', 'numerals', 'year_numerals', 'link', 'own', 'tmdb'));
