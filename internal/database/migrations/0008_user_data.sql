-- What each user did with each movie, episode or other item: whether it was
-- played and how often, where playback stopped, and whether the user marked
-- it as a favorite or rated it. Series and seasons are counted from their
-- episodes, which name them.
CREATE TABLE user_data (
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    item_id uuid NOT NULL,
    series_id uuid,
    season_id uuid,
    played boolean NOT NULL DEFAULT false,
    play_count integer NOT NULL DEFAULT 0 CHECK (play_count >= 0),
    -- The resume point, and the runtime it was measured against, in
    -- Jellyfin ticks (100 ns).
    position_ticks bigint NOT NULL DEFAULT 0 CHECK (position_ticks >= 0),
    runtime_ticks bigint NOT NULL DEFAULT 0 CHECK (runtime_ticks >= 0),
    favorite boolean NOT NULL DEFAULT false,
    -- The user's rating from 0 to 10: a like is stored as 10, a dislike as 1.
    rating double precision CHECK (rating BETWEEN 0 AND 10),
    last_played_at timestamptz,
    PRIMARY KEY (user_id, item_id)
);

CREATE INDEX user_data_resumable ON user_data (user_id, last_played_at DESC) WHERE position_ticks > 0;
CREATE INDEX user_data_series ON user_data (user_id, series_id) WHERE series_id IS NOT NULL;
CREATE INDEX user_data_favorites ON user_data (user_id) WHERE favorite;
