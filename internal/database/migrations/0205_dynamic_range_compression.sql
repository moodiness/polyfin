-- How converted AC-3 and E-AC-3 apply the dynamic range compression their
-- tracks carry: encoded, as the track says and as before; none, the full
-- range; or night, quiet passages lifted twice as much. Direct play and
-- copied audio are left to the app's decoder.
ALTER TABLE settings
    ADD COLUMN dynamic_range_compression text NOT NULL DEFAULT 'encoded'
        CHECK (dynamic_range_compression IN ('encoded', 'none', 'night'));
