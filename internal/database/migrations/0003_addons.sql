-- Stremio addons, installed from their manifest URL. An addon without owner
-- is shared by the server; otherwise it belongs to one user. Manifest URLs
-- usually embed the user's credentials for the addon.
CREATE TABLE addons (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id uuid REFERENCES users (id) ON DELETE CASCADE,
    manifest_url text NOT NULL,
    manifest jsonb NOT NULL CHECK (jsonb_typeof(manifest) = 'object'),
    enabled boolean NOT NULL DEFAULT true,
    position integer NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    refreshed_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX addons_unique_url
    ON addons (coalesce(owner_id, '00000000-0000-0000-0000-000000000000'), manifest_url);
CREATE INDEX addons_owner ON addons (owner_id, position);

-- Catalogs shown as libraries. A row means the catalog is enabled; position
-- orders the libraries of the addon owner's scope.
CREATE TABLE libraries (
    addon_id uuid NOT NULL REFERENCES addons (id) ON DELETE CASCADE,
    catalog_type text NOT NULL,
    catalog_id text NOT NULL,
    name text CHECK (name IS NULL OR length(name) BETWEEN 1 AND 64),
    position integer NOT NULL,
    PRIMARY KEY (addon_id, catalog_type, catalog_id)
);

-- Whether a user sees the server's addons in addition to their own.
ALTER TABLE users ADD COLUMN use_shared_addons boolean NOT NULL DEFAULT true;
