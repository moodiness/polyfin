-- The libraries whose catalog the RefreshSources task found missing from
-- their addon's manifest, and when it first did. The task keeps such a
-- library, storing nothing of that manifest, until a run at least
-- addons.RemovalConfirmation later finds the catalog still missing: an
-- addon's passing failure may answer a manifest without some catalogs,
-- and one look alone removes no library. A manifest stored, by the task
-- or by Refresh on the source's page, which removes such libraries at
-- once, deletes its addon's rows. The table has no trigger: holding a
-- manifest changes nothing apps see.
CREATE TABLE missing_catalogs (
    addon_id uuid NOT NULL,
    catalog_type text NOT NULL,
    catalog_id text NOT NULL,
    since timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (addon_id, catalog_type, catalog_id),
    FOREIGN KEY (addon_id, catalog_type, catalog_id) REFERENCES libraries ON DELETE CASCADE ON UPDATE CASCADE
);
