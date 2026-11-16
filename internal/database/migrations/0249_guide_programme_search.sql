-- Programme searches match the folded title of XMLTV guides' programmes,
-- as Polyfin folds names (lower case, without accents: iptv.Fold), with
-- LIKE: Polyfin writes it as it stores a guide. It is NULL where it is not
-- written yet, as for the programmes stored before, which Polyfin folds
-- in the background, and before any search reads them (see
-- library.Service.foldGuideTitles). A trigram index serves the searches
-- when the server has pg_trgm (see search_indexes); without it they read
-- the guide's programmes and test the column, only slower.
ALTER TABLE live_guide_programmes ADD COLUMN search text;

CREATE INDEX live_guide_programmes_unfolded ON live_guide_programmes (guide_id) WHERE search IS NULL;

DO $$
DECLARE
    trgm name;
BEGIN
    SELECT n.nspname INTO trgm FROM pg_extension e JOIN pg_namespace n ON n.oid = e.extnamespace WHERE e.extname = 'pg_trgm';
    IF trgm IS NOT NULL THEN
        BEGIN
            EXECUTE format($sql$CREATE INDEX IF NOT EXISTS live_guide_programmes_search ON live_guide_programmes
                USING gin (search %I.gin_trgm_ops)$sql$, trgm);
        EXCEPTION WHEN OTHERS THEN
            RAISE WARNING 'The programme search index could not be created: %', SQLERRM;
        END;
    END IF;
END
$$;
