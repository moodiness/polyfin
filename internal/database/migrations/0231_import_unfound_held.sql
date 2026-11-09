-- An episode a server import (Jellyfin, Emby or Plex) did not find keeps
-- how its server holds it, so that a link made by hand places it among
-- the addon's episodes as the import does, rather than by its numbers
-- alone: its own TMDB and TVDB identifiers, the time it aired, its name,
-- and whether its series is shown numbered otherwise (see
-- importlinks.Held). Null for the other entries.
ALTER TABLE import_unfound ADD COLUMN held jsonb;
