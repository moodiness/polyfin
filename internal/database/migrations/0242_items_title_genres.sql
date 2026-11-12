-- The genres Polyfin keeps of each movie and series, so that a genre's page
-- finds its titles without reading every item: those of the preview a
-- catalog gave (meta) and those of the complete description one of the
-- server's addons gave (genres), trimmed, in lower case, each once.
-- A list of names folds the same way as {"genres": names}.
--
-- Letter case is folded under the builtin collation pg_c_utf8, whose
-- Unicode case mapping is the same on every server, whatever the
-- database's locale, its provider or the version of the system's
-- collation library. A server older than PostgreSQL 17, or a database of
-- another encoding than UTF-8, has no such collation: genres then fold
-- under "C", ASCII letters only. The choice is written into the function
-- once, here, and stays with it, its index and the backups that hold
-- them. The function reads nothing but its argument and names only
-- pg_catalog's functions and operators, so it is immutable.
DO $$
BEGIN
    EXECUTE format($function$
        CREATE FUNCTION polyfin_title_genres(data jsonb) RETURNS text[]
        LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $body$
            SELECT coalesce(array_agg(DISTINCT folded ORDER BY folded), '{}') FROM (
                SELECT lower(btrim(genre) COLLATE %s) AS folded
                FROM jsonb_array_elements_text(
                    CASE WHEN jsonb_typeof(data->'genres') = 'array' THEN data->'genres' ELSE '[]' END
                    || CASE WHEN jsonb_typeof(data->'meta'->'genres') = 'array' THEN data->'meta'->'genres' ELSE '[]' END) AS genre) AS genres
            WHERE folded <> ''
        $body$
    $function$, CASE WHEN getdatabaseencoding() = 'UTF8'
        AND EXISTS (SELECT FROM pg_collation WHERE collname = 'pg_c_utf8' AND collnamespace = 'pg_catalog'::regnamespace)
        THEN 'pg_c_utf8' ELSE '"C"' END);
END
$$;

CREATE INDEX items_title_genres ON items USING gin (polyfin_title_genres(data)) WHERE kind IN ('movie', 'series');
