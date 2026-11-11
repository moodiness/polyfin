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
--
-- Adding a key back checks every row, and some databases hold rows their
-- key does not match, though it was checked when added. An index on text
-- built under another collation version, as after an update of the
-- system's collation library, may not follow the order PostgreSQL now
-- compares in, and a search through it misses rows: a library or reach
-- deleted can leave behind the rows its cascade did not find. Those rows
-- go before each key is added back: guides and channel choices of a
-- library deleted, and arrivals of a reach forgotten.
--
-- For the rest of this migration, nothing reads through an index, so that
-- neither the deletes nor the checks of the keys miss a row that is there.
-- The primary keys are not rebuilt instead: such an index may also have
-- let in a second row of the same key, and rebuilding it would fail.
SET LOCAL enable_indexscan = off;
SET LOCAL enable_indexonlyscan = off;
SET LOCAL enable_bitmapscan = off;

DELETE FROM live_guides g WHERE NOT EXISTS (SELECT FROM libraries l
    WHERE l.addon_id = g.addon_id AND l.catalog_type = g.catalog_type AND l.catalog_id = g.catalog_id);

ALTER TABLE live_guides
    DROP CONSTRAINT live_guides_addon_id_catalog_type_catalog_id_fkey,
    ADD CONSTRAINT live_guides_addon_id_catalog_type_catalog_id_fkey
        FOREIGN KEY (addon_id, catalog_type, catalog_id) REFERENCES libraries ON UPDATE CASCADE ON DELETE CASCADE;

DELETE FROM live_guide_maps m WHERE NOT EXISTS (SELECT FROM libraries l
    WHERE l.addon_id = m.addon_id AND l.catalog_type = m.catalog_type AND l.catalog_id = m.catalog_id);

ALTER TABLE live_guide_maps
    DROP CONSTRAINT live_guide_maps_addon_id_catalog_type_catalog_id_fkey,
    ADD CONSTRAINT live_guide_maps_addon_id_catalog_type_catalog_id_fkey
        FOREIGN KEY (addon_id, catalog_type, catalog_id) REFERENCES libraries ON UPDATE CASCADE ON DELETE CASCADE;

DELETE FROM catalog_arrivals a WHERE NOT EXISTS (SELECT FROM catalog_reach r
    WHERE r.addon_id = a.addon_id AND r.catalog_type = a.catalog_type AND r.catalog_id = a.catalog_id AND r.genre = a.genre);

ALTER TABLE catalog_arrivals
    DROP CONSTRAINT catalog_arrivals_addon_id_catalog_type_catalog_id_genre_fkey,
    ADD CONSTRAINT catalog_arrivals_addon_id_catalog_type_catalog_id_genre_fkey
        FOREIGN KEY (addon_id, catalog_type, catalog_id, genre) REFERENCES catalog_reach ON UPDATE CASCADE ON DELETE CASCADE;
