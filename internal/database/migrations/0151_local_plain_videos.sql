-- The files a local movies or shows folder leaves unmatched are listed in
-- its library as plain videos. plain_id is the identifier of the plain
-- video a file's unit is while it is unmatched, made of the folder's and
-- the unit's (see localfiles.plainID), written by each scan; null until
-- the file is stored again.
ALTER TABLE local_files ADD COLUMN plain_id text CHECK (plain_id <> '');
CREATE INDEX local_files_plain ON local_files (plain_id) WHERE plain_id IS NOT NULL;

-- The movies and shows folders describe and stream their plain videos'
-- identifiers too (see localfiles.manifest).
UPDATE addons a SET manifest = jsonb_set(a.manifest, '{resources}', (
    SELECT jsonb_agg(CASE WHEN r->>'name' IN ('meta', 'stream') AND jsonb_typeof(r->'idPrefixes') = 'array'
            AND NOT (r->'idPrefixes') ? 'polyfin-plain:'
        THEN jsonb_set(r, '{idPrefixes}', (r->'idPrefixes') || '"polyfin-plain:"'::jsonb) ELSE r END ORDER BY e.n)
    FROM jsonb_array_elements(a.manifest->'resources') WITH ORDINALITY AS e(r, n)))
FROM local_folders f
WHERE f.addon_id = a.id AND f.kind IN ('movies', 'shows') AND jsonb_typeof(a.manifest->'resources') = 'array';
