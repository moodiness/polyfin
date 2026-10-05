-- Whether each user imports the watch history of a tracking service they
-- connected (off by default), and how the last import went: when it ended,
-- the played marks and resume points it added, the titles of the history
-- Polyfin could not identify, and what stopped it reading the history
-- whole. cursor is where the service's history stood at the last import
-- that read it whole, as the service tells it, for the connection made at
-- cursor_for: the next import reads only what changed since. The row
-- outlives a connection made again; disconnecting deletes it.
CREATE TABLE tracking_imports (
    user_id uuid NOT NULL REFERENCES users ON DELETE CASCADE,
    service text NOT NULL CHECK (service IN ('trakt', 'simkl', 'mdblist', 'publicmetadb')),
    enabled boolean NOT NULL DEFAULT false,
    last_import_at timestamptz,
    played integer NOT NULL DEFAULT 0 CHECK (played >= 0),
    resumed integer NOT NULL DEFAULT 0 CHECK (resumed >= 0),
    unmapped integer NOT NULL DEFAULT 0 CHECK (unmapped >= 0),
    problem text CHECK (problem IN ('reconnect', 'unreachable', 'rate_limited')),
    cursor text NOT NULL DEFAULT '',
    cursor_for timestamptz,
    PRIMARY KEY (user_id, service)
);
