-- Smart lists: lists of titles drawn from catalogs of addons, which keep the
-- titles that match their rules (see package smartlists). owner_id is NULL
-- for the server's lists, which every user sees, else the user whose own
-- list it is, which only they see. catalogs names the catalogs a list draws
-- from ([{"addonId", "type", "id"}]), and rules its rules, a group of rules
-- and groups. shown_as tells whether apps see it as a library or as a
-- collection; sort how its titles are ordered, and max_items the most it
-- lists, NULL for no maximum.
CREATE TABLE smart_lists (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id uuid REFERENCES users ON DELETE CASCADE,
    name text NOT NULL CHECK (name <> '' AND length(name) <= 64),
    shown_as text NOT NULL CHECK (shown_as IN ('library', 'collection')),
    catalogs jsonb NOT NULL,
    rules jsonb NOT NULL,
    sort text NOT NULL CHECK (sort IN ('catalog', 'premiere', 'rating', 'name', 'random')),
    max_items integer CHECK (max_items BETWEEN 1 AND 20000),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX smart_lists_owner ON smart_lists (owner_id, created_at);
