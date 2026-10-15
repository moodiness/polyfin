-- User groups hold the limits of several accounts at once: the settings of
-- a user's page an administrator sets for each user, but administrator
-- status, the sign-in screen, the disabled switch and the end date. The
-- columns are those of users, with the defaults of a new user.
CREATE TABLE user_groups (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (name <> ''),
    created_at timestamptz NOT NULL DEFAULT now(),
    max_parental_rating integer,
    max_parental_sub_rating integer,
    block_unrated_items text[] NOT NULL DEFAULT '{}',
    video_transcoding boolean NOT NULL DEFAULT true,
    audio_transcoding boolean NOT NULL DEFAULT true,
    content_downloading boolean NOT NULL DEFAULT true,
    personal_addons boolean NOT NULL DEFAULT true,
    max_playbacks integer NOT NULL DEFAULT 0 CHECK (max_playbacks BETWEEN 0 AND 20),
    max_bitrate integer NOT NULL DEFAULT 0 CHECK (max_bitrate >= 0),
    live_tv boolean NOT NULL DEFAULT true,
    sync_play text NOT NULL DEFAULT 'CreateAndJoinGroups'
        CHECK (sync_play IN ('CreateAndJoinGroups', 'JoinGroups', 'None')),
    remote_control boolean NOT NULL DEFAULT false,
    hidden_libraries uuid[] NOT NULL DEFAULT '{}',
    blocked_genres text[] NOT NULL DEFAULT '{}' CHECK (cardinality(blocked_genres) <= 100),
    access_schedules jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(access_schedules) = 'array'
        AND jsonb_array_length(access_schedules) <= 50),
    collection_management boolean NOT NULL DEFAULT false,
    subtitle_management boolean NOT NULL DEFAULT false,
    live_tv_management boolean NOT NULL DEFAULT false,
    quality_group integer NOT NULL DEFAULT 0 CHECK (quality_group IN (0, 480, 720, 1080, 1440, 2160)),
    can_share boolean NOT NULL DEFAULT false
);
CREATE UNIQUE INDEX user_groups_name ON user_groups (lower(name));

-- A user belongs to one group at most, never an administrator. While
-- follow_group is set, their own columns hold the group's values, written
-- with each change to the group; turned off, they keep their own. Without
-- a group it stays set, the default for the next group they join.
ALTER TABLE users
    ADD COLUMN group_id uuid REFERENCES user_groups (id) ON DELETE SET NULL,
    ADD COLUMN follow_group boolean NOT NULL DEFAULT true,
    ADD CONSTRAINT users_administrator_group CHECK (NOT (is_administrator AND group_id IS NOT NULL));
CREATE INDEX users_group ON users (group_id) WHERE group_id IS NOT NULL;

-- The group the accounts an invite creates join and follow.
ALTER TABLE invites ADD COLUMN group_id uuid REFERENCES user_groups (id) ON DELETE SET NULL;
