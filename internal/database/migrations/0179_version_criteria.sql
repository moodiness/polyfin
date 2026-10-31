-- The version order gains the criteria size-per-minute, source tier,
-- audio format, edition and label score, and up to six criteria. The
-- edition criterion orders the editions as version_edition_order lists
-- them; size-per-minute puts first the versions closest to
-- version_mb_per_minute megabytes a minute, the largest a minute first
-- for 0. The label rules score or hide versions by their labels: each a
-- word or a regular expression, with a score or hiding; none by default.
ALTER TABLE settings
    DROP CONSTRAINT settings_version_order_check,
    ADD CONSTRAINT settings_version_order_check CHECK (cardinality(version_order) <= 6
        AND version_order <@ '{language,resolution-high,resolution-low,size-large,size-small,size-per-minute,codec,hdr-first,hdr-last,source,tier,audio,edition,labels}'),
    ADD COLUMN version_edition_order text[] NOT NULL DEFAULT '{theatrical,extended,directors,imax}'
        CHECK (version_edition_order <@ '{theatrical,extended,directors,imax}'),
    ADD COLUMN version_mb_per_minute integer NOT NULL DEFAULT 0 CHECK (version_mb_per_minute BETWEEN 0 AND 1000),
    ADD COLUMN version_label_rules jsonb NOT NULL DEFAULT '[]'
        CHECK (jsonb_typeof(version_label_rules) = 'array' AND jsonb_array_length(version_label_rules) <= 40);
