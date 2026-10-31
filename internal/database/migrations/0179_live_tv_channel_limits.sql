-- The Live TV channels of the server's sources a user, or the members of a
-- group, may see: mode "all", "only" (the groups and channels listed) or
-- "except" (all but those). groups are channel groups (an IPTV category,
-- or an addon catalog's genre) and channels are channels, by the stable
-- keys their sources give them. With hideNewGroups, "except" also hides
-- the groups not among knownGroups, those the server had when the rule
-- was saved. Every user and group sees every channel until limited.
ALTER TABLE users ADD COLUMN live_tv_channels jsonb NOT NULL DEFAULT '{"mode": "all"}'
    CHECK (jsonb_typeof(live_tv_channels) = 'object' AND live_tv_channels->>'mode' IN ('all', 'only', 'except'));
ALTER TABLE user_groups ADD COLUMN live_tv_channels jsonb NOT NULL DEFAULT '{"mode": "all"}'
    CHECK (jsonb_typeof(live_tv_channels) = 'object' AND live_tv_channels->>'mode' IN ('all', 'only', 'except'));
