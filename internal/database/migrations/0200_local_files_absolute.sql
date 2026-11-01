-- The absolute episode number a show's file is named by, as "Show -
-- 41.mkv", NULL for a file named otherwise; and how its season and
-- episode were told: '' by its name, 'absolute' from its absolute number,
-- the catalog's episode of that place, 'absolute_beyond' when the catalog
-- does not reach it, the file then being an episode of its own.
ALTER TABLE local_files
    ADD COLUMN absolute integer,
    ADD COLUMN numbering text NOT NULL DEFAULT '';
