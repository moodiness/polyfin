-- Whether the web player adds the versions the addons find to a title's
-- page while it is open, rather than only replacing the placeholder.
ALTER TABLE settings
    ADD COLUMN add_versions_to_open_page boolean NOT NULL DEFAULT true;
