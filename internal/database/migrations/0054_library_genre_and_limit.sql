-- A library may be narrowed to one of the genres its catalog offers in its
-- genre filter, and may list at most max_items titles, and as many in each
-- of its collections, in place of the server's catalog limit. NULL lists
-- the whole catalog, up to that limit, as before. Live TV catalogs take
-- neither.
ALTER TABLE libraries
    ADD COLUMN genre text CHECK (genre IS NULL OR (genre <> '' AND catalog_type <> 'tv')),
    ADD COLUMN max_items integer CHECK (max_items IS NULL OR (max_items BETWEEN 1 AND 20000 AND catalog_type <> 'tv'));
