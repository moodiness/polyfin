-- Polyfin keeps the addons, libraries and guides of every scope in memory
-- (see addons.Store). Any change to them, whoever makes it, tells the
-- server to read them again once its transaction commits; PostgreSQL sends
-- one notification per transaction.
CREATE FUNCTION notify_addons_changed() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_notify('polyfin_addons', '');
    RETURN NULL;
END
$$;
CREATE TRIGGER addons_changed AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON addons
    FOR EACH STATEMENT EXECUTE FUNCTION notify_addons_changed();
CREATE TRIGGER libraries_changed AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON libraries
    FOR EACH STATEMENT EXECUTE FUNCTION notify_addons_changed();
CREATE TRIGGER live_guides_changed AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON live_guides
    FOR EACH STATEMENT EXECUTE FUNCTION notify_addons_changed();
