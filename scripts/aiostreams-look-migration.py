#!/usr/bin/env python3
"""Writes the migration that makes internal/accounts/aiostreams_theme.css, as
it is now, the web player's default custom CSS: on new servers, and on
servers whose custom CSS still holds the look as the last migration setting
it left it, with no custom script. A released migration never changes, so
after each change of the CSS this writes a new one:

    scripts/aiostreams-look-migration.py 0147_aiostreams_look_mobile
"""
import pathlib, re, sys

root = pathlib.Path(__file__).resolve().parent.parent
mig = root / "internal/database/migrations"
name = sys.argv[1]
css = (root / "internal/accounts/aiostreams_theme.css").read_text()
assert "$css$" not in css
# The CSS the last migration made the default.
last = None
for path in sorted(mig.glob("*.sql")):
    if path.stem == name:
        continue
    found = re.search(r"ALTER COLUMN custom_css SET DEFAULT \$css\$(.*?)\$css\$", path.read_text(), re.S)
    if found:
        last = found.group(1)
assert last is not None and last != css, "nothing to change"
(mig / (name + ".sql")).write_text(f"""-- The AIOStreams look of the web player, as it is now (see
-- internal/accounts/aiostreams_theme.css), becomes the default custom CSS:
-- on new servers, and on servers that still hold the look as the last
-- migration setting it left it, without a custom script. A server that
-- changed either keeps both as they are.
ALTER TABLE settings ALTER COLUMN custom_css SET DEFAULT $css${css}$css$;
UPDATE settings SET custom_css = DEFAULT
    WHERE custom_css = $css${last}$css$
    AND custom_js = '';
""")
