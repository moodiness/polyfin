-- The cast and crew of movies and series as TMDB credits them, while the
-- settings hold the server's TMDB key: by the identifier the title was
-- looked up with (its TMDB identifier, else its IMDb one), for every user.
-- tmdb_id is the title's TMDB identifier, NULL for a title TMDB does not
-- know. credits are the people kept, in the order shown, each with their
-- TMDB identifier, name, role, kind and the path of their photo on TMDB's
-- image server. language is the language the roles were asked in. titles
-- are the items whose people were saved from these credits, which also
-- maps those titles to their TMDB identifier. A lookup that failed leaves
-- no row.
CREATE TABLE title_credits (
    kind text NOT NULL CHECK (kind IN ('movie', 'tv')),
    provider text NOT NULL CHECK (provider IN ('imdb', 'tmdb')),
    id text NOT NULL,
    tmdb_id integer CHECK (tmdb_id > 0),
    language text NOT NULL DEFAULT '',
    credits jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(credits) = 'array'),
    titles uuid[] NOT NULL DEFAULT '{}',
    asked_at timestamptz NOT NULL,
    PRIMARY KEY (kind, provider, id)
);

CREATE INDEX title_credits_tmdb ON title_credits (kind, tmdb_id) WHERE tmdb_id IS NOT NULL;

-- The movies and series TMDB credits each person in, by the person's TMDB
-- identifier: a list of {"kind": "movie" or "tv", "id": TMDB identifier}.
-- found is false for a person TMDB does not know.
CREATE TABLE person_credits (
    tmdb_id integer PRIMARY KEY CHECK (tmdb_id > 0),
    found boolean NOT NULL,
    titles jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(titles) = 'array'),
    asked_at timestamptz NOT NULL
);

-- The TMDB identifier of the movies box sets looked up, NULL when TMDB does
-- not know the movie or it was looked up before it was kept.
ALTER TABLE movie_collections ADD COLUMN tmdb_id integer;

CREATE INDEX movie_collections_tmdb ON movie_collections (tmdb_id) WHERE tmdb_id IS NOT NULL;

-- polyfin_merged_person is the record of the person TMDB identifies as
-- tmdb_id, from the one kept under that identifier (target, NULL when there
-- is none) and one of theirs kept under their name (source):
-- - their name, and their photo, are the target's, else the source's;
-- - their credits are those of both, a title credited in both keeping the
--   credits of each;
-- - what TMDB told of them is the target's when it is of tmdb_id, else the
--   source's when it is, with the biographies of both when both are; what
--   TMDB told of someone else is dropped, so that the person is looked up
--   again by tmdb_id;
-- - they are confined only when both are, and photos found dead on either
--   stay so.
CREATE FUNCTION polyfin_merged_person(target jsonb, source jsonb, tmdb_id text) RETURNS jsonb
LANGUAGE sql IMMUTABLE AS $$
    SELECT jsonb_strip_nulls(jsonb_build_object(
        'kind', 'person',
        'person', jsonb_strip_nulls(jsonb_build_object(
            'name', COALESCE(target->'person'->'name', source->'person'->'name'),
            'image', COALESCE(target->'person'->'image', source->'person'->'image'))),
        'tmdbId', tmdb_id,
        'credits', (SELECT COALESCE(jsonb_object_agg(title, kinds), '{}'::jsonb) FROM (
            SELECT title, jsonb_agg(DISTINCT kind) AS kinds FROM (
                SELECT credit.key AS title, kind FROM jsonb_each(COALESCE(source->'credits', '{}'::jsonb)) AS credit,
                    jsonb_array_elements(credit.value) AS kind
                UNION ALL
                SELECT credit.key, kind FROM jsonb_each(COALESCE(target->'credits', '{}'::jsonb)) AS credit,
                    jsonb_array_elements(credit.value) AS kind) AS all_credits
            GROUP BY title) AS merged),
        'tmdb', CASE
            WHEN target->'tmdb'->>'id' = tmdb_id AND source->'tmdb'->>'id' = tmdb_id THEN
                target->'tmdb' || jsonb_build_object('biographies',
                    COALESCE(source->'tmdb'->'biographies', '{}'::jsonb) || COALESCE(target->'tmdb'->'biographies', '{}'::jsonb))
            WHEN target->'tmdb'->>'id' = tmdb_id THEN target->'tmdb'
            WHEN source->'tmdb'->>'id' = tmdb_id THEN source->'tmdb'
        END,
        'confined', CASE WHEN target IS NULL THEN source->'confined'
            WHEN COALESCE((target->>'confined')::boolean, false) AND COALESCE((source->>'confined')::boolean, false) THEN 'true'::jsonb
        END,
        'deadPhotos', NULLIF(COALESCE(source->'deadPhotos', '{}'::jsonb) || COALESCE(target->'deadPhotos', '{}'::jsonb), '{}'::jsonb)))
$$;

-- polyfin_person_tmdb_id is the identifier of the item of the person TMDB
-- identifies as tmdb_id, derived from its key as Polyfin derives every
-- item's.
CREATE FUNCTION polyfin_person_tmdb_id(tmdb_id text) RETURNS uuid
LANGUAGE sql IMMUTABLE AS $$
    SELECT encode(substring(sha256(convert_to('polyfin:item:persontmdb|' || tmdb_id, 'UTF8')) FROM 1 FOR 16), 'hex')::uuid
$$;

-- People were kept by name. Those an addon gave a TMDB identifier move to
-- the item of that identifier, merged when several names have the same
-- one; each name keeps a record pointing to it (sameAs), so that its
-- identifier, links to it and credits by that name alone still reach the
-- person. The users' marks, an administrator's edits and uploaded images
-- move with them. People of no known identifier stay kept by name.
CREATE TEMPORARY TABLE moved_people ON COMMIT DROP AS
    SELECT id AS old_id, data, data->>'tmdbId' AS tmdb_id, polyfin_person_tmdb_id(data->>'tmdbId') AS new_id
    FROM items
    WHERE kind = 'person' AND key LIKE 'person|%' AND data->>'tmdbId' ~ '^[1-9][0-9]{0,9}$' AND NOT data ? 'sameAs';

DO $$
DECLARE
    moved record;
BEGIN
    FOR moved IN SELECT * FROM moved_people ORDER BY new_id, old_id LOOP
        INSERT INTO items (id, key, kind, data)
            VALUES (moved.new_id, 'persontmdb|' || moved.tmdb_id, 'person', polyfin_merged_person(NULL, moved.data, moved.tmdb_id))
            ON CONFLICT (id) DO UPDATE SET data = polyfin_merged_person(items.data, moved.data, moved.tmdb_id), updated_at = now();
    END LOOP;
END $$;

UPDATE items SET data = jsonb_build_object('kind', 'person', 'person', jsonb_build_object('name', items.data->'person'->'name'),
        'sameAs', moved.new_id::text, 'tmdbId', moved.tmdb_id), updated_at = now()
    FROM moved_people AS moved WHERE items.id = moved.old_id;

-- Of several records moving to one person, the marks, edits and images of
-- the first are kept, those the person already has winning.
UPDATE user_data SET item_id = pick.new_id FROM (
        SELECT DISTINCT ON (marked.user_id, moved.new_id) marked.user_id, marked.item_id, moved.new_id
        FROM user_data AS marked JOIN moved_people AS moved ON marked.item_id = moved.old_id
        WHERE NOT EXISTS (SELECT 1 FROM user_data AS kept WHERE kept.user_id = marked.user_id AND kept.item_id = moved.new_id)
        ORDER BY marked.user_id, moved.new_id, marked.favorite DESC, marked.item_id) AS pick
    WHERE user_data.user_id = pick.user_id AND user_data.item_id = pick.item_id;

UPDATE item_overrides SET item_id = pick.new_id FROM (
        SELECT DISTINCT ON (moved.new_id) edited.item_id, moved.new_id
        FROM item_overrides AS edited JOIN moved_people AS moved ON edited.item_id = moved.old_id
        WHERE NOT EXISTS (SELECT 1 FROM item_overrides AS kept WHERE kept.item_id = moved.new_id)
        ORDER BY moved.new_id, edited.item_id) AS pick
    WHERE item_overrides.item_id = pick.item_id;

UPDATE item_images SET item_id = pick.new_id FROM (
        SELECT DISTINCT ON (moved.new_id, uploaded.image_type) uploaded.item_id, uploaded.image_type, moved.new_id
        FROM item_images AS uploaded JOIN moved_people AS moved ON uploaded.item_id = moved.old_id
        WHERE NOT EXISTS (SELECT 1 FROM item_images AS kept WHERE kept.item_id = moved.new_id AND kept.image_type = uploaded.image_type)
        ORDER BY moved.new_id, uploaded.image_type, uploaded.item_id) AS pick
    WHERE item_images.item_id = pick.item_id AND item_images.image_type = pick.image_type;
