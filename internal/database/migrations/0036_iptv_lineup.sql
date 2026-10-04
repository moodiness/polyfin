-- IPTV line-ups: an IPTV source's provider list (its entries) is turned,
-- by its import options, into the line-up the administrator edits:
-- categories, channels and the streams of each channel.
ALTER TABLE iptv_channels RENAME TO iptv_entries;
ALTER INDEX iptv_channels_order RENAME TO iptv_entries_order;

-- Import options: provider groups or one category per country; one
-- channel per entry, or the entries of the same name in a category merged
-- into one channel; the group (g:<title>) and country (c:<code>) keys not
-- imported; whether channels appearing on a refresh arrive enabled; and
-- whether a channel without a fixed number takes the provider's.
-- lineup_at is when the line-up was last reconciled with the list, NULL
-- until it was; included_groups, the groups shown before line-ups existed,
-- only disables the other categories on that first reconciliation.
ALTER TABLE iptv_sources
    ADD COLUMN category_mode text NOT NULL DEFAULT 'original' CHECK (category_mode IN ('original', 'country')),
    ADD COLUMN channel_mode text NOT NULL DEFAULT 'original' CHECK (channel_mode IN ('original', 'merged')),
    ADD COLUMN excluded text[] NOT NULL DEFAULT '{}' CHECK (cardinality(excluded) <= 5000),
    ADD COLUMN new_channels boolean NOT NULL DEFAULT true,
    ADD COLUMN numbering text NOT NULL DEFAULT 'provider' CHECK (numbering IN ('provider', 'sequential')),
    ADD COLUMN lineup_at timestamptz,
    DROP COLUMN groups;

-- The categories of a line-up, in position order: provider ones keyed by
-- group or country, the administrator's own (custom) by u:<id>. name is the
-- administrator's name, NULL for the provider's.
CREATE TABLE iptv_categories (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    addon_id uuid NOT NULL REFERENCES iptv_sources ON DELETE CASCADE,
    key text NOT NULL CHECK (key <> ''),
    provider_name text NOT NULL DEFAULT '',
    name text CHECK (name IS NULL OR length(name) BETWEEN 1 AND 64),
    position integer NOT NULL CHECK (position > 0),
    enabled boolean NOT NULL DEFAULT true,
    custom boolean NOT NULL DEFAULT false,
    UNIQUE (addon_id, key),
    CHECK (NOT custom OR name IS NOT NULL)
);

-- The channels of a line-up. id never changes and is part of the channel's
-- Stremio identifier, item_id its Jellyfin identifier; key is what the
-- line-up is derived by (an entry, or a category and a name when merged)
-- and may change when the channel is adopted by another derivation.
-- category_id is its provider category, moved_to the category the
-- administrator put it in. Within a category, channels are ordered by
-- sort: the provider's order, unless sort_set (moved by the administrator).
-- name, logo and number are the administrator's, NULL for the provider's;
-- search holds the folded names and guide id that searches match.
CREATE TABLE iptv_lineup (
    addon_id uuid NOT NULL REFERENCES iptv_sources ON DELETE CASCADE,
    id text NOT NULL CHECK (id <> ''),
    item_id uuid NOT NULL UNIQUE,
    key text NOT NULL CHECK (key <> ''),
    category_id uuid NOT NULL REFERENCES iptv_categories ON DELETE CASCADE,
    moved_to uuid REFERENCES iptv_categories ON DELETE SET NULL,
    provider_name text NOT NULL,
    name text CHECK (name IS NULL OR length(name) BETWEEN 1 AND 100),
    provider_logo text NOT NULL DEFAULT '',
    logo text CHECK (logo IS NULL OR length(logo) <= 4096),
    description text NOT NULL DEFAULT '' CHECK (length(description) <= 2000),
    guide_id text NOT NULL DEFAULT '',
    provider_number integer CHECK (provider_number > 0),
    number integer CHECK (number BETWEEN 1 AND 99999),
    sort double precision NOT NULL,
    sort_set boolean NOT NULL DEFAULT false,
    enabled boolean NOT NULL DEFAULT true,
    search text NOT NULL DEFAULT '',
    PRIMARY KEY (addon_id, id),
    -- Reconciling swaps keys between channels within one statement.
    UNIQUE (addon_id, key) DEFERRABLE INITIALLY DEFERRED
);
CREATE INDEX iptv_lineup_order ON iptv_lineup (addon_id, category_id, sort);
CREATE INDEX iptv_lineup_moved ON iptv_lineup (moved_to) WHERE moved_to IS NOT NULL;

-- The streams of a channel: a provider entry (key is the entry's), or a
-- custom stream (key u:<id>, with its address). label is its quality, rank
-- the provider order of its channel's streams (best first), sort the
-- administrator's order, NULL until set.
CREATE TABLE iptv_streams (
    addon_id uuid NOT NULL,
    key text NOT NULL CHECK (key <> ''),
    channel_id text NOT NULL,
    label text NOT NULL CHECK (length(label) BETWEEN 1 AND 32),
    rank integer NOT NULL DEFAULT 0,
    sort integer,
    enabled boolean NOT NULL DEFAULT true,
    custom_url text CHECK (custom_url IS NULL OR length(custom_url) <= 4096),
    PRIMARY KEY (addon_id, key),
    FOREIGN KEY (addon_id, channel_id) REFERENCES iptv_lineup ON DELETE CASCADE ON UPDATE CASCADE,
    CHECK ((custom_url IS NOT NULL) = (key LIKE 'u:%'))
);
CREATE INDEX iptv_streams_channel ON iptv_streams (addon_id, channel_id);

-- The XMLTV guides of a live TV catalog, in position order. generation
-- names the channels and programmes of the last successful download;
-- channels and programmes count them; checked_at is the last attempt,
-- fetched_at the last success, error the code of the last failure.
CREATE TABLE live_guides (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    addon_id uuid NOT NULL,
    catalog_type text NOT NULL,
    catalog_id text NOT NULL,
    position integer NOT NULL CHECK (position > 0),
    url text NOT NULL CHECK (url <> '' AND length(url) <= 4096),
    generation integer NOT NULL DEFAULT 0,
    checked_at timestamptz,
    fetched_at timestamptz,
    channels integer NOT NULL DEFAULT 0 CHECK (channels >= 0),
    programmes integer NOT NULL DEFAULT 0 CHECK (programmes >= 0),
    error text NOT NULL DEFAULT ''
        CHECK (error IN ('', 'unreachable', 'private_network', 'too_large', 'malformed', 'channels_unreachable')),
    FOREIGN KEY (addon_id, catalog_type, catalog_id) REFERENCES libraries ON DELETE CASCADE,
    CHECK (catalog_type = 'tv')
);
CREATE INDEX live_guides_catalog ON live_guides (addon_id, catalog_type, catalog_id, position);

-- The channels and programmes of a guide's downloads, by generation; search
-- holds its folded names and id.
CREATE TABLE live_guide_channels (
    guide_id uuid NOT NULL REFERENCES live_guides ON DELETE CASCADE,
    generation integer NOT NULL,
    xmltv_id text NOT NULL,
    names text[] NOT NULL DEFAULT '{}',
    icon text NOT NULL DEFAULT '',
    search text NOT NULL DEFAULT '',
    PRIMARY KEY (guide_id, generation, xmltv_id)
);

CREATE TABLE live_guide_programmes (
    guide_id uuid NOT NULL REFERENCES live_guides ON DELETE CASCADE,
    generation integer NOT NULL,
    xmltv_id text NOT NULL,
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    title text NOT NULL CHECK (title <> ''),
    subtitle text NOT NULL DEFAULT '',
    description text NOT NULL DEFAULT '',
    categories text[] NOT NULL DEFAULT '{}',
    season integer CHECK (season > 0),
    episode integer CHECK (episode > 0),
    icon text NOT NULL DEFAULT '',
    CHECK (ends_at > starts_at)
);
CREATE INDEX live_guide_programmes_channel ON live_guide_programmes (guide_id, generation, xmltv_id, starts_at);

-- Which guide channel gives a catalog's channel its programmes: channel_id
-- is the channel's Jellyfin item id. manual marks a choice made by hand,
-- which only a remap changes; a manual row without a guide channel pins
-- "no guide".
CREATE TABLE live_guide_maps (
    addon_id uuid NOT NULL,
    catalog_type text NOT NULL,
    catalog_id text NOT NULL,
    channel_id uuid NOT NULL,
    guide_id uuid REFERENCES live_guides ON DELETE CASCADE,
    xmltv_id text,
    manual boolean NOT NULL DEFAULT false,
    PRIMARY KEY (addon_id, catalog_type, catalog_id, channel_id),
    FOREIGN KEY (addon_id, catalog_type, catalog_id) REFERENCES libraries ON DELETE CASCADE,
    CHECK ((guide_id IS NULL) = (xmltv_id IS NULL)),
    CHECK (manual OR guide_id IS NOT NULL)
);
CREATE INDEX live_guide_maps_channel ON live_guide_maps (channel_id);

-- Today's guide addresses become the first guide of their catalog, fetched
-- again at once: their programmes were kept by channel, not by guide.
INSERT INTO live_guides (addon_id, catalog_type, catalog_id, position, url)
    SELECT addon_id, catalog_type, catalog_id, 1, guide_url FROM libraries WHERE guide_url <> '' AND catalog_type = 'tv';
DROP TABLE guide_programmes;
ALTER TABLE libraries
    DROP COLUMN guide_url,
    DROP COLUMN guide_checked_at,
    DROP COLUMN guide_fetched_at,
    DROP COLUMN guide_error;
-- guide_channels and guide_matched now count the catalog's channels and
-- those mapped to a guide channel.
UPDATE libraries SET guide_channels = 0, guide_matched = 0;
