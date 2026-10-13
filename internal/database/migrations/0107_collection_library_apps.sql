-- The names of the apps, as they send them in the Client field of their
-- authorization, told an addon's collection library holds collections
-- (boxsets) rather than mixed content; at most 32.
ALTER TABLE settings
    ADD COLUMN collection_library_apps text[] NOT NULL DEFAULT '{}' CHECK (cardinality(collection_library_apps) <= 32);
