-- The version order gains two criteria: the versions an addon says are
-- cached by their provider first, and the most seeders first.
ALTER TABLE settings
    DROP CONSTRAINT settings_version_order_check,
    ADD CONSTRAINT settings_version_order_check CHECK (cardinality(version_order) <= 6
        AND version_order <@ '{language,resolution-high,resolution-low,size-large,size-small,size-per-minute,codec,hdr-first,hdr-last,source,tier,audio,edition,labels,cached,seeders}');
