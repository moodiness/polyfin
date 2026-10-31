-- Who uses a server addon: everyone, the default, or only the groups and
-- users addon_users lists (listed), which may leave every administrator
-- out. A user's own addons are theirs alone, and stay used by everyone.
-- Deleting a group or a user removes them from the lists; an addon whose
-- list empties is used by no one, never by everyone.
ALTER TABLE addons
    ADD COLUMN used_by text NOT NULL DEFAULT 'everyone' CHECK (used_by IN ('everyone', 'listed')),
    ADD CONSTRAINT addons_used_by_server CHECK (used_by = 'everyone' OR owner_id IS NULL);

CREATE TABLE addon_users (
    addon_id uuid NOT NULL REFERENCES addons ON DELETE CASCADE,
    group_id uuid REFERENCES user_groups ON DELETE CASCADE,
    user_id uuid REFERENCES users ON DELETE CASCADE,
    CHECK ((group_id IS NULL) <> (user_id IS NULL))
);
CREATE UNIQUE INDEX addon_users_group ON addon_users (addon_id, group_id) WHERE group_id IS NOT NULL;
CREATE UNIQUE INDEX addon_users_user ON addon_users (addon_id, user_id) WHERE user_id IS NOT NULL;
CREATE INDEX addon_users_by_group ON addon_users (group_id) WHERE group_id IS NOT NULL;
CREATE INDEX addon_users_by_user ON addon_users (user_id) WHERE user_id IS NOT NULL;

-- The lists are part of the addons the server keeps in memory (see
-- addons.Store): a change to them, a group or user deleted included,
-- tells it to read them again.
CREATE TRIGGER addon_users_changed AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON addon_users
    FOR EACH STATEMENT EXECUTE FUNCTION notify_addons_changed();
