-- The XMLTV guide of a live TV catalog: its address, which may embed the
-- user's credentials, and how it was last fetched. checked_at is the last
-- attempt, fetched_at the last success; channels counts the catalog's
-- channels then, matched those the guide covers; error is the code of the
-- last failure, empty after a success.
ALTER TABLE libraries
    ADD COLUMN guide_url text NOT NULL DEFAULT ''
        CHECK (length(guide_url) <= 4096 AND (guide_url = '' OR catalog_type = 'tv')),
    ADD COLUMN guide_checked_at timestamptz,
    ADD COLUMN guide_fetched_at timestamptz,
    ADD COLUMN guide_channels integer NOT NULL DEFAULT 0 CHECK (guide_channels >= 0),
    ADD COLUMN guide_matched integer NOT NULL DEFAULT 0,
    ADD COLUMN guide_error text NOT NULL DEFAULT ''
        CHECK (guide_error IN ('', 'unreachable', 'private_network', 'too_large', 'malformed', 'channels_unreachable')),
    ADD CONSTRAINT libraries_guide_matched CHECK (guide_matched BETWEEN 0 AND guide_channels);

-- The programmes of the channels an XMLTV guide covers, by the item
-- identifier of the channel, from a day ago to eight days ahead when the
-- guide was fetched. Season and episode count from 1.
CREATE TABLE guide_programmes (
    channel_id uuid NOT NULL,
    addon_id uuid NOT NULL,
    catalog_type text NOT NULL,
    catalog_id text NOT NULL,
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    title text NOT NULL CHECK (title <> ''),
    subtitle text NOT NULL DEFAULT '',
    description text NOT NULL DEFAULT '',
    categories text[] NOT NULL DEFAULT '{}',
    season integer CHECK (season > 0),
    episode integer CHECK (episode > 0),
    icon text NOT NULL DEFAULT '',
    PRIMARY KEY (channel_id, addon_id, catalog_type, catalog_id, starts_at),
    FOREIGN KEY (addon_id, catalog_type, catalog_id) REFERENCES libraries ON DELETE CASCADE,
    CHECK (ends_at > starts_at)
);
CREATE INDEX guide_programmes_catalog ON guide_programmes (addon_id, catalog_type, catalog_id);
