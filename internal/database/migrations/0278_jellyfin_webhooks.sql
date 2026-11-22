-- A kind of target more: 'jellyfin' receives the JSON of Jellyfin's
-- webhook plugin. Like a webhook, it keeps the scheme and host of its
-- address in address, and in secret its address, then, on a second line
-- when it has some, the headers sent with it, as JSON. Its events are the
-- plugin's notification types, such as 'PlaybackStart'.
--
-- user_filter lists the users whose events a server's Jellyfin target
-- receives, NULL for every user's; events of no user are always sent.
ALTER TABLE notification_targets
    DROP CONSTRAINT notification_targets_kind_check,
    DROP CONSTRAINT notification_targets_address_check,
    ADD CONSTRAINT notification_targets_kind_check
        CHECK (kind IN ('webhook', 'jellyfin', 'discord', 'ntfy', 'email', 'telegram', 'gotify', 'pushover')),
    ADD CONSTRAINT notification_targets_address_check CHECK (CASE kind
        WHEN 'email' THEN address LIKE '%_@_%'
        WHEN 'telegram' THEN address = ''
        WHEN 'pushover' THEN address = ''
        ELSE address LIKE 'http://%' OR address LIKE 'https://%' END),
    ADD COLUMN user_filter uuid[] CHECK (cardinality(user_filter) BETWEEN 1 AND 1000),
    ADD CONSTRAINT notification_targets_user_filter_owner_check CHECK (user_filter IS NULL OR user_id IS NULL AND kind = 'jellyfin');
