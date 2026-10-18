-- The criterion source orders a title's versions by their source: the
-- sources version_source_order lists first, in its order, then the others
-- in the addons' order. It lists the server's sources, addons and folders,
-- by ID, each once; a source removed since is left out when read.
ALTER TABLE settings
    DROP CONSTRAINT settings_version_order_check,
    ADD CONSTRAINT settings_version_order_check CHECK (cardinality(version_order) <= 4
        AND version_order <@ '{language,resolution-high,resolution-low,size-large,size-small,codec,hdr-first,hdr-last,source}'),
    ADD COLUMN version_source_order uuid[] NOT NULL DEFAULT '{}' CHECK (cardinality(version_source_order) <= 500);
