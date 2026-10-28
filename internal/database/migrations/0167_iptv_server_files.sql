-- An administrator's M3U list or XMLTV guide may be a file of the server,
-- stored by its path (see serverfile.Path). One that is missing or that
-- Polyfin may not read failed with file_unreadable; one of a source or a
-- catalog that may only reach public addresses, with file_refused.
ALTER TABLE iptv_sources
    DROP CONSTRAINT iptv_sources_error_check,
    ADD CONSTRAINT iptv_sources_error_check
        CHECK (error IN ('', 'unreachable', 'private_network', 'too_large', 'malformed', 'rate_limited',
            'file_unreadable', 'file_refused'));

ALTER TABLE live_guides
    DROP CONSTRAINT live_guides_error_check,
    ADD CONSTRAINT live_guides_error_check
        CHECK (error IN ('', 'unreachable', 'private_network', 'too_large', 'malformed', 'channels_unreachable', 'rate_limited',
            'file_unreadable', 'file_refused'));
