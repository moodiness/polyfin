-- Each addon's last stream list for a title, so that a restart lists the
-- versions known before at once, as a stale list, while the addon is asked
-- again. answer is how many of the streams the addon's last answer gave,
-- the others being kept from the list it replaced while it is followed up.
CREATE TABLE stream_lists (
    addon_id uuid NOT NULL REFERENCES addons (id) ON DELETE CASCADE,
    content_type text NOT NULL,
    stremio_id text NOT NULL,
    streams jsonb NOT NULL,
    answer integer NOT NULL,
    fetched_at timestamptz NOT NULL,
    PRIMARY KEY (addon_id, content_type, stremio_id)
);
CREATE INDEX stream_lists_fetched ON stream_lists (fetched_at);
