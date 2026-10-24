-- A local folder's file a search matched by a looser rule tells which (see
-- localfiles.chooseItem): "year" for a title of the year before or after
-- the one its name gives, "numerals" for a title named alike once numerals
-- are read, "year_numerals" for both.
ALTER TABLE local_files
    DROP CONSTRAINT local_files_matched_by_check,
    ADD CONSTRAINT local_files_matched_by_check
        CHECK (matched_by IN ('', 'name', 'nfo', 'search', 'year', 'numerals', 'year_numerals', 'link', 'own'));

-- The units of a local folder (see local_files) an administrator unlinked
-- from the title a looser rule matched: their files are matched by the
-- exact rule only, through every scan.
CREATE TABLE local_exact_units (
    addon_id uuid NOT NULL REFERENCES local_folders ON DELETE CASCADE,
    unit text NOT NULL CHECK (unit <> ''),
    PRIMARY KEY (addon_id, unit)
);
