-- The rows of Streamyfin's home screen, in order, which Polyfin serves as
-- the settings of Streamyfin's server plugin (/Streamyfin/config): the
-- user's titles to resume, their next episodes, a library's items, or a
-- collection's titles. item_id is the library or the collection, and
-- item_name its name when the row was saved, shown while it is gone; title
-- is the administrator's, NULL for the default name. With no row,
-- Streamyfin shows its own home screen.
CREATE TABLE streamyfin_rows (
    position integer PRIMARY KEY CHECK (position >= 1),
    kind text NOT NULL CHECK (kind IN ('resume', 'next_up', 'library', 'collection')),
    item_id uuid,
    item_name text NOT NULL DEFAULT '',
    title text CHECK (title IS NULL OR length(title) BETWEEN 1 AND 64),
    CHECK ((kind IN ('resume', 'next_up')) = (item_id IS NULL))
);
