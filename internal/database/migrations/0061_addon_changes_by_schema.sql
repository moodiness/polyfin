-- A change to the addons, libraries or guides is notified with the schema
-- of the table changed. The channel is the database's own: servers, or
-- tests, sharing a database each keep their tables in a schema of their
-- own, and each follows only the changes of its schema (see addons.Store).
CREATE OR REPLACE FUNCTION notify_addons_changed() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_notify('polyfin_addons', TG_TABLE_SCHEMA);
    RETURN NULL;
END
$$;
