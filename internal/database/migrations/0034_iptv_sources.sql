-- How many hours after it was fetched an XMLTV guide or an IPTV source's
-- channel list is fetched again; 12 was the fixed interval before.
ALTER TABLE settings
    ADD COLUMN live_tv_refresh_hours integer NOT NULL DEFAULT 12 CHECK (live_tv_refresh_hours BETWEEN 1 AND 168);

-- An addon is a Stremio addon, or one of Polyfin's own IPTV sources: an M3U
-- playlist or an Xtream Codes account, whose address, embedding its
-- credentials, is the manifest URL, and whose manifest describes its one
-- live TV catalog.
ALTER TABLE addons
    ADD COLUMN kind text NOT NULL DEFAULT 'stremio' CHECK (kind IN ('stremio', 'm3u', 'xtream'));

-- An IPTV source's channel list: the groups it had when last fetched, in
-- order; the groups whose channels are shown, NULL for all of them, those
-- added later included; and how its last fetch went. checked_at is the last
-- attempt, fetched_at the last success, error the code of the last failure,
-- empty after a success.
CREATE TABLE iptv_sources (
    addon_id uuid PRIMARY KEY REFERENCES addons (id) ON DELETE CASCADE,
    groups text[] NOT NULL DEFAULT '{}',
    included_groups text[],
    checked_at timestamptz,
    fetched_at timestamptz,
    error text NOT NULL DEFAULT ''
        CHECK (error IN ('', 'unreachable', 'private_network', 'too_large', 'malformed'))
);

-- The channels of an IPTV source's list, in its order: key identifies the
-- channel within the source; number is the provider's, empty when it gives
-- none; guide_id its channel in XMLTV guides; url, which may embed the
-- credentials, and headers how its stream is requested.
CREATE TABLE iptv_channels (
    addon_id uuid NOT NULL REFERENCES iptv_sources ON DELETE CASCADE,
    key text NOT NULL CHECK (key <> ''),
    position integer NOT NULL CHECK (position > 0),
    name text NOT NULL CHECK (name <> ''),
    number integer CHECK (number > 0),
    logo text NOT NULL DEFAULT '',
    group_title text NOT NULL DEFAULT '',
    guide_id text NOT NULL DEFAULT '',
    url text NOT NULL CHECK (url <> ''),
    headers jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(headers) = 'object'),
    PRIMARY KEY (addon_id, key)
);
CREATE INDEX iptv_channels_order ON iptv_channels (addon_id, position);
