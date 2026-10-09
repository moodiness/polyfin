-- The API account an administrator creates on Last.fm, through which users
-- connect their Last.fm accounts: its API key and shared secret. Empty by
-- default, which offers Last.fm to nobody. Printable ASCII without spaces,
-- as Last.fm issues them; the secret is stored sealed with
-- POLYFIN_SECRET_KEY like the server's other keys, hence its larger bound.
ALTER TABLE settings
    ADD COLUMN lastfm_api_key text NOT NULL DEFAULT '' CHECK (octet_length(lastfm_api_key) <= 256 AND lastfm_api_key ~ '^[!-~]*$'),
    ADD COLUMN lastfm_secret text NOT NULL DEFAULT '' CHECK (octet_length(lastfm_secret) <= 512 AND lastfm_secret ~ '^[!-~]*$');

-- Users connect Last.fm (its session key in token) and ListenBrainz (their
-- user token) to have the songs they play scrobbled. Their scrobbles wait in
-- tracking_events with the rest, title naming the song's track item.
ALTER TABLE tracking_connections
    DROP CONSTRAINT tracking_connections_service_check,
    ADD CONSTRAINT tracking_connections_service_check
        CHECK (service IN ('trakt', 'simkl', 'mdblist', 'publicmetadb', 'lastfm', 'listenbrainz'));
