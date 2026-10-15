-- The apps an administrator registers with AniList and MyAnimeList, through
-- which users connect their accounts: each app's client ID and secret.
-- Empty by default, which offers neither service. Printable ASCII without
-- spaces, as the services issue them; the secrets are stored sealed with
-- POLYFIN_SECRET_KEY like the server's other keys, hence their larger
-- bound.
ALTER TABLE settings
    ADD COLUMN anilist_client_id text NOT NULL DEFAULT '' CHECK (octet_length(anilist_client_id) <= 256 AND anilist_client_id ~ '^[!-~]*$'),
    ADD COLUMN anilist_client_secret text NOT NULL DEFAULT '' CHECK (octet_length(anilist_client_secret) <= 512 AND anilist_client_secret ~ '^[!-~]*$'),
    ADD COLUMN myanimelist_client_id text NOT NULL DEFAULT '' CHECK (octet_length(myanimelist_client_id) <= 256 AND myanimelist_client_id ~ '^[!-~]*$'),
    ADD COLUMN myanimelist_client_secret text NOT NULL DEFAULT '' CHECK (octet_length(myanimelist_client_secret) <= 512 AND myanimelist_client_secret ~ '^[!-~]*$');

-- Users connect AniList and MyAnimeList to have the episodes and movies they
-- watch raise their lists' progress, and import those lists.
ALTER TABLE tracking_connections
    DROP CONSTRAINT tracking_connections_service_check,
    ADD CONSTRAINT tracking_connections_service_check
        CHECK (service IN ('trakt', 'simkl', 'mdblist', 'publicmetadb', 'lastfm', 'listenbrainz', 'anilist', 'myanimelist'));

-- Whether each user syncs their ratings with AniList or MyAnimeList (off by
-- default): the ratings they give are sent as their lists' scores, and the
-- imports give their unrated titles the lists' scores. The last import
-- counts the titles it rated, and skipped the ratings of series spread over
-- several entries of the lists, which the ratings sent since add to.
ALTER TABLE tracking_imports
    DROP CONSTRAINT tracking_imports_service_check,
    ADD CONSTRAINT tracking_imports_service_check
        CHECK (service IN ('trakt', 'simkl', 'mdblist', 'publicmetadb', 'anilist', 'myanimelist')),
    ADD COLUMN ratings boolean NOT NULL DEFAULT false,
    ADD COLUMN rated integer NOT NULL DEFAULT 0 CHECK (rated >= 0),
    ADD COLUMN ratings_skipped integer NOT NULL DEFAULT 0 CHECK (ratings_skipped >= 0);
