-- How a list says a channel's stream is encrypted, as its #KODIPROP lines
-- tell it: the manifest and licence types, and for ClearKey the keys or
-- the licence server that gives them. The keys are the list's own data,
-- kept with it and nowhere else. NULL when the list says nothing.
ALTER TABLE iptv_entries
    ADD COLUMN protection jsonb CHECK (jsonb_typeof(protection) = 'object');

-- A stream may fail as one that needs a DRM Polyfin does not support
-- (drm), or whose list's ClearKey keys do not decrypt it (keys).
ALTER TABLE iptv_streams
    DROP CONSTRAINT iptv_streams_failure_check,
    ADD CONSTRAINT iptv_streams_failure_check
        CHECK (failure IN ('dead', 'refused', 'timeout', 'drm', 'keys'));
