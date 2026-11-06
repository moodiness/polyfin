-- The app of a user's last activity, written with last_activity_at: the
-- client name a Jellyfin or Silo app sent, or the admin app's own name.
-- Empty when unknown. Users active before it count their most recently
-- active device's app.
ALTER TABLE users ADD COLUMN last_activity_app text NOT NULL DEFAULT '';

UPDATE users u SET last_activity_app = d.client
FROM (
    SELECT DISTINCT ON (user_id) user_id, client
    FROM devices
    ORDER BY user_id, last_activity_at DESC, id
) d
WHERE d.user_id = u.id AND u.last_activity_at IS NOT NULL;
