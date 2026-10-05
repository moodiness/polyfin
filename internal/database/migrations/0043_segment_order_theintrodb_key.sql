-- The TheIntroDB API key the server asks for skip segments with, empty
-- while none is saved, which asks TheIntroDB without one; printable ASCII,
-- the size in bytes. And the order of preference of the segment databases:
-- each of them once, or empty to follow POLYFIN_SEGMENTS.
ALTER TABLE settings
    ADD COLUMN theintrodb_key text NOT NULL DEFAULT '' CHECK (octet_length(theintrodb_key) <= 256 AND theintrodb_key ~ '^[ -~]*$'),
    ADD COLUMN segment_order text[] NOT NULL DEFAULT '{}' CHECK (
        segment_order = '{}' OR (
            cardinality(segment_order) = 3 AND segment_order @> ARRAY['theintrodb', 'introdb', 'publicmetadb']
        )
    );
