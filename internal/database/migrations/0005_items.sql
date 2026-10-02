-- Jellyfin apps address libraries, collections, titles, seasons and episodes
-- by identifier. Each identifier is derived from a stable key (for example
-- the title's Stremio ID), so a title keeps its identifier across catalogs
-- and restarts; this table finds what an identifier designates, with the
-- last data seen for it.
CREATE TABLE items (
    id uuid PRIMARY KEY,
    key text NOT NULL UNIQUE,
    kind text NOT NULL,
    data jsonb NOT NULL CHECK (jsonb_typeof(data) = 'object'),
    updated_at timestamptz NOT NULL DEFAULT now()
);
