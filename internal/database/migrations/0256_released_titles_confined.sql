-- A title whose addon is removed, or whose catalog no longer exists, is
-- released: its record loses its addon and keeps the description that
-- addon gave, artwork addresses included. A release now keeps confined
-- with it, which tells whether those addresses are downloaded from public
-- addresses only. The releases made before dropped it, and which addon a
-- record was released from is no longer known: every title released
-- before is confined, until a catalog lists it again, which replaces its
-- description and confined together.
--
-- A title no catalog ever listed has no addon either. Its record holds its
-- name and identifiers alone, no address, and stays as it is: its artwork
-- is its describing addon's.
UPDATE items SET data = data || '{"confined": true}'::jsonb, updated_at = now()
WHERE kind IN ('movie', 'series', 'video')
    AND NOT data ? 'addon'
    AND NOT data @> '{"confined": true}'::jsonb
    AND CASE WHEN jsonb_typeof(data->'meta') = 'object'
        THEN (data->'meta') - '{id,type,name,imdb_id,_tmdbId,_tvdbId}'::text[] <> '{}'::jsonb
        ELSE false END;
