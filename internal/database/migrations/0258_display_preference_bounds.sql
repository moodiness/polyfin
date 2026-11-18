-- Display preferences get bounds. Apps keep a few kilobytes under a
-- client's name of a few letters, as jellyfin-web does under "emby"; the
-- table kept a value and a name of any size.
--
-- Rows over a bound are removed whole rather than cut: a value cut short
-- is no longer JSON, and a name cut short is another client's. Their user
-- then gets, for that view and that app, what a user who never saved any
-- gets.
DELETE FROM display_preferences WHERE octet_length(value::text) > 131072 OR octet_length(client) > 64;

-- The value's bound is on the text the database prints of it, which puts a
-- space after each colon and comma: twice the 64 KiB Polyfin accepts
-- (preferences.MaxValueBytes), which no value it writes reaches.
--
-- updated_at is when a row was last written: past the number of rows a
-- user keeps (preferences.MaxPerUser), a new one takes the place of the one
-- written longest ago. The rows from before were all written now.
ALTER TABLE display_preferences
    ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now(),
    ADD CONSTRAINT display_preferences_value_size CHECK (octet_length(value::text) <= 131072),
    ADD CONSTRAINT display_preferences_client_size CHECK (octet_length(client) <= 64);
