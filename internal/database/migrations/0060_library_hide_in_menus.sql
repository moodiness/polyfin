-- A library may be left out of the web player's menus: its top bar, the
-- bar's More menu and its side menu. It keeps its home screen row, and
-- other apps keep listing it. Live TV catalogs make no menu entry: their
-- channels go to Live TV.
ALTER TABLE libraries
    ADD COLUMN hide_in_menus boolean NOT NULL DEFAULT false CHECK (NOT hide_in_menus OR catalog_type <> 'tv');
