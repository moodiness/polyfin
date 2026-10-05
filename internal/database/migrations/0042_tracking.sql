-- The apps an administrator registers with Trakt and Simkl, through which
-- users connect their accounts: Trakt's client ID and secret, Simkl's
-- client ID. Empty by default, which offers neither service. Printable
-- ASCII without spaces, as the services issue them.
ALTER TABLE settings
    ADD COLUMN trakt_client_id text NOT NULL DEFAULT '' CHECK (octet_length(trakt_client_id) <= 256 AND trakt_client_id ~ '^[!-~]*$'),
    ADD COLUMN trakt_client_secret text NOT NULL DEFAULT '' CHECK (octet_length(trakt_client_secret) <= 256 AND trakt_client_secret ~ '^[!-~]*$'),
    ADD COLUMN simkl_client_id text NOT NULL DEFAULT '' CHECK (octet_length(simkl_client_id) <= 256 AND simkl_client_id ~ '^[!-~]*$');

-- The tracking services each user connected: the token (with its refresh
-- token and expiry, for the services that issue them) or the API key
-- Polyfin sends what they watch with, the account name the service gave,
-- and how sending goes. problem is 'reconnect' once the service refused
-- the token or key, 'unreachable' while recent sends fail; failures counts
-- the sends that failed in a row.
CREATE TABLE tracking_connections (
    user_id uuid NOT NULL REFERENCES users ON DELETE CASCADE,
    service text NOT NULL CHECK (service IN ('trakt', 'simkl', 'mdblist', 'publicmetadb')),
    token text NOT NULL CHECK (token <> ''),
    refresh_token text NOT NULL DEFAULT '',
    expires_at timestamptz,
    account text,
    connected_at timestamptz NOT NULL,
    last_sent_at timestamptz,
    problem text CHECK (problem IN ('reconnect', 'unreachable')),
    failures integer NOT NULL DEFAULT 0 CHECK (failures >= 0),
    PRIMARY KEY (user_id, service)
);

-- What waits to be sent to a connected service, oldest first: changes to
-- the user's history and resume points, kept until the service accepts
-- them or they are too old to retry. kind names the request and payload
-- holds what it describes; title is the item a resume point is for, so
-- that a newer one replaces it. Disconnecting drops them.
CREATE TABLE tracking_events (
    id bigserial PRIMARY KEY,
    user_id uuid NOT NULL,
    service text NOT NULL,
    kind text NOT NULL,
    title uuid,
    payload jsonb NOT NULL,
    created_at timestamptz NOT NULL,
    attempts integer NOT NULL DEFAULT 0,
    FOREIGN KEY (user_id, service) REFERENCES tracking_connections ON DELETE CASCADE
);
CREATE INDEX tracking_events_lane ON tracking_events (user_id, service, id);

-- When each service last counted a title watched for the user, by
-- Polyfin's identifier of the movie or episode: a played mark sent soon
-- after would add a second viewing to the service's history.
CREATE TABLE tracking_watched (
    user_id uuid NOT NULL,
    service text NOT NULL,
    title uuid NOT NULL,
    watched_at timestamptz NOT NULL,
    PRIMARY KEY (user_id, service, title),
    FOREIGN KEY (user_id, service) REFERENCES tracking_connections ON DELETE CASCADE
);
