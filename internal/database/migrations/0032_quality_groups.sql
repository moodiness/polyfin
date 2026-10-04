-- Quality groups: the tallest video a user is offered, as a height in
-- lines, 0 for Original, no limit. Taller versions are left out while one
-- fits, and converted down to the group when played.
ALTER TABLE users
    ADD COLUMN quality_group integer NOT NULL DEFAULT 0 CHECK (quality_group IN (0, 480, 720, 1080, 1440, 2160));
