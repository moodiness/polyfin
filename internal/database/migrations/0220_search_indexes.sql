-- Name searches (people, episodes, and the titles share links find) match
-- a lower-case substring with LIKE: trigram indexes serve them when the
-- server has pg_trgm. A server without it, or a role that may not create
-- it, keeps searching without them, only slower: nothing here fails an
-- upgrade. The extension goes in public, where a database's extensions
-- usually are, so that it stays when another schema is dropped; the
-- current schema when public cannot take it.
DO $$
DECLARE
    trgm name;
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_trgm')
        AND EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'pg_trgm') THEN
        BEGIN
            CREATE EXTENSION IF NOT EXISTS pg_trgm SCHEMA public;
        EXCEPTION WHEN OTHERS THEN
            BEGIN
                CREATE EXTENSION IF NOT EXISTS pg_trgm;
            EXCEPTION WHEN OTHERS THEN
                RAISE WARNING 'pg_trgm could not be created, so name searches run without their indexes: %', SQLERRM;
            END;
        END;
    END IF;
    SELECT n.nspname INTO trgm FROM pg_extension e JOIN pg_namespace n ON n.oid = e.extnamespace WHERE e.extname = 'pg_trgm';
    IF trgm IS NOT NULL THEN
        BEGIN
            EXECUTE format($sql$CREATE INDEX IF NOT EXISTS items_person_name_trgm ON items
                USING gin (lower(data->'person'->>'name') %I.gin_trgm_ops) WHERE kind = 'person'$sql$, trgm);
            EXECUTE format($sql$CREATE INDEX IF NOT EXISTS items_episode_name_trgm ON items
                USING gin (lower(coalesce(nullif(data->'video'->>'name', ''), data->'video'->>'title', '')) %I.gin_trgm_ops)
                WHERE kind = 'episode'$sql$, trgm);
            EXECUTE format($sql$CREATE INDEX IF NOT EXISTS items_title_name_trgm ON items
                USING gin (lower(data->'meta'->>'name') %I.gin_trgm_ops) WHERE kind IN ('movie', 'series')$sql$, trgm);
        EXCEPTION WHEN OTHERS THEN
            RAISE WARNING 'The name search indexes could not be created: %', SQLERRM;
        END;
    END IF;
END
$$;

-- People are listed by name: from a name on, before one, and a page at a
-- time.
CREATE INDEX IF NOT EXISTS items_person_name ON items ((lower(data->'person'->>'name')), id) WHERE kind = 'person';

-- Refresh ratings reads the titles whose ratings were asked.
CREATE INDEX IF NOT EXISTS items_rated ON items (id) WHERE data ? 'ratedAt';
