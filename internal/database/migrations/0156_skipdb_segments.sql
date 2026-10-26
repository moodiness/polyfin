-- SkipDB, a fourth segment database. Existing servers get it last in their
-- order of preference, turned on, unless every other database is turned
-- off: a server that asks none keeps asking none.
ALTER TABLE settings
    DROP CONSTRAINT settings_segment_order_check,
    DROP CONSTRAINT settings_segment_sources_off_check;
UPDATE settings SET
    segment_order = segment_order || ARRAY['skipdb'],
    segment_sources_off = CASE WHEN segment_sources_off @> ARRAY['theintrodb', 'introdb', 'publicmetadb']
        THEN segment_sources_off || ARRAY['skipdb'] ELSE segment_sources_off END;
ALTER TABLE settings
    ADD CONSTRAINT settings_segment_order_check CHECK (
        cardinality(segment_order) = 4 AND segment_order @> ARRAY['theintrodb', 'introdb', 'publicmetadb', 'skipdb']
    ),
    ALTER COLUMN segment_order SET DEFAULT ARRAY['theintrodb', 'introdb', 'publicmetadb', 'skipdb'],
    ADD CONSTRAINT settings_segment_sources_off_check CHECK (
        segment_sources_off <@ ARRAY['theintrodb', 'introdb', 'publicmetadb', 'skipdb']
        AND cardinality(array_positions(segment_sources_off, 'theintrodb')) <= 1
        AND cardinality(array_positions(segment_sources_off, 'introdb')) <= 1
        AND cardinality(array_positions(segment_sources_off, 'publicmetadb')) <= 1
        AND cardinality(array_positions(segment_sources_off, 'skipdb')) <= 1
    );
