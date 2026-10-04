-- Each user's playback and access limits, as Jellyfin's user policy holds
-- them: how many of their devices may play at once and the highest bitrate
-- they play at (0 for no limit for both), whether they reach Live TV, what
-- they may do in SyncPlay, and whether they may control other users' apps.
ALTER TABLE users
    ADD COLUMN max_playbacks integer NOT NULL DEFAULT 0 CHECK (max_playbacks BETWEEN 0 AND 20),
    ADD COLUMN max_bitrate integer NOT NULL DEFAULT 0 CHECK (max_bitrate >= 0),
    ADD COLUMN live_tv boolean NOT NULL DEFAULT true,
    ADD COLUMN sync_play text NOT NULL DEFAULT 'CreateAndJoinGroups'
        CHECK (sync_play IN ('CreateAndJoinGroups', 'JoinGroups', 'None')),
    ADD COLUMN remote_control boolean NOT NULL DEFAULT false;

-- Administrators controlled other users' apps until now; they keep doing so.
UPDATE users SET remote_control = is_administrator;
