-- The library of a catalog whose type or identifier changes keeps the
-- identifier apps know it by (see addons.Addon.LibraryKeys): library_keys
-- lists the catalogs, by their current type and identifier, whose library
-- takes its identifier from another key. It replaces library_key_types,
-- the type each library kept, by catalog identifier only, which named the
-- first catalog of that identifier in the manifest.
ALTER TABLE addons ADD COLUMN library_keys jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(library_keys) = 'array');

UPDATE addons a SET library_keys = (
    SELECT coalesce(jsonb_agg(jsonb_build_object(
            'catalog', jsonb_build_object('type', c.type, 'id', k.key),
            'key', jsonb_build_object('type', k.value #>> '{}', 'id', k.key)) ORDER BY k.key), '[]')
    FROM jsonb_each(a.library_key_types) k
    CROSS JOIN LATERAL (
        SELECT e->>'type' AS type
        FROM jsonb_array_elements(CASE WHEN jsonb_typeof(a.manifest->'catalogs') = 'array' THEN a.manifest->'catalogs' ELSE '[]' END)
            WITH ORDINALITY m(e, n)
        WHERE e->>'id' = k.key
        ORDER BY n
        LIMIT 1) c
    WHERE c.type <> k.value #>> '{}')
WHERE library_key_types <> '{}';

ALTER TABLE addons DROP COLUMN library_key_types;

-- A catalog's guides, their channel choices and programmes follow its
-- library when the catalog is renamed, and so does the arrival history of
-- the titles of its pages.
ALTER TABLE live_guides
    DROP CONSTRAINT live_guides_addon_id_catalog_type_catalog_id_fkey,
    ADD CONSTRAINT live_guides_addon_id_catalog_type_catalog_id_fkey
        FOREIGN KEY (addon_id, catalog_type, catalog_id) REFERENCES libraries ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE live_guide_maps
    DROP CONSTRAINT live_guide_maps_addon_id_catalog_type_catalog_id_fkey,
    ADD CONSTRAINT live_guide_maps_addon_id_catalog_type_catalog_id_fkey
        FOREIGN KEY (addon_id, catalog_type, catalog_id) REFERENCES libraries ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE catalog_arrivals
    DROP CONSTRAINT catalog_arrivals_addon_id_catalog_type_catalog_id_genre_fkey,
    ADD CONSTRAINT catalog_arrivals_addon_id_catalog_type_catalog_id_genre_fkey
        FOREIGN KEY (addon_id, catalog_type, catalog_id, genre) REFERENCES catalog_reach ON UPDATE CASCADE ON DELETE CASCADE;
