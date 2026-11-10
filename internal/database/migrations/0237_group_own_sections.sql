-- A member follows their group section by section: own_sections lists
-- the sections of their page in which they keep their own values, those
-- of accounts.GroupSections; in the others, their columns hold the group's
-- values. Without a group it is empty. A member who no longer followed
-- their group keeps every section as their own.
ALTER TABLE users
    ADD COLUMN own_sections text[] NOT NULL DEFAULT '{}'
        CHECK (own_sections <@ ARRAY['access', 'playback', 'parental', 'libraries', 'genres', 'hours', 'channels']::text[]);
UPDATE users SET own_sections = ARRAY['access', 'playback', 'parental', 'libraries', 'genres', 'hours', 'channels']
    WHERE group_id IS NOT NULL AND NOT follow_group;
ALTER TABLE users DROP COLUMN follow_group;
