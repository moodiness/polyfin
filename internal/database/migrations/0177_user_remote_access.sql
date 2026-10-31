-- Whether a user may sign in and use the server from outside the local
-- network, Jellyfin's EnableRemoteAccess: on for every user and every
-- group, as Jellyfin gives it to new users, so that nothing changes until
-- an administrator takes it away.
ALTER TABLE users ADD COLUMN remote_access boolean NOT NULL DEFAULT true;
ALTER TABLE user_groups ADD COLUMN remote_access boolean NOT NULL DEFAULT true;
